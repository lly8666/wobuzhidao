//go:build windows

package windowsclient

import (
    "context"
    "errors"
    "os"
)

// n4OwnedOrphanGuard permits a fake WFP backend ONLY in isolated unit tests.
// The exported recovery entry point below always constructs the real,
// deterministic installation-owned persistent WFP guard.
type n4OwnedOrphanGuard interface {
    ownedState(context.Context) (N4UnifiedState, error)
    DisarmAfterRoutesRemoved(context.Context) error
}

// N4WindowsRetireOwnedGuardOrphanWAL addresses exactly one more crash window:
// the original created Wintun is gone and the durable parent WAL contains
// ONLY "guard:owned". Sequential five-stage Apply guarantees no later stage
// was entered without an additional fsynced WAL intent. In particular, this
// is NEVER general recovery of lease/MTU/DNS/route state.
//
// It may remove ONLY positively identified installation-owned WFP rules,
// and only when the old OS adapter GUID/LUID/index are all absent and there
// is no native per-route WAL. An altered rule, foreign sublayer, ambiguous
// ownership, cancellation, or uncertain deletion keeps the parent WAL.
// Never open/recreate the original Wintun, enumerate/delete foreign routes,
// or adopt a same-name adapter.
func N4WindowsRetireOwnedGuardOrphanWAL(ctx context.Context, path string, id N4UnifiedIdentity) error {
    g := &n4WindowsPersistentWFPGuard{
        InstallationID: id.Installation,
        WintunLUID: id.WintunLUID,
        ExclusiveSingleWintun: true,
    }
    // The flag above is NOT an Arm permission: Arm remains permanently
    // disabled. This is an exact-ID, cleanup-only recovery of an existing
    // legacy owned WFP sublayer, not installation of a new global policy.
    return n4RetireOwnedGuardOrphanWAL(ctx, path, id, g)
}

func n4RetireOwnedGuardOrphanWAL(ctx context.Context, path string, id N4UnifiedIdentity, g n4OwnedOrphanGuard) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    if path == "" || !id.valid() || id.WintunGUID == "" ||
        id.AdapterAlias == "" || g == nil {
        return ErrN4JournalConflict
    }
    release, err := (N4WindowsMachineGate{}).Acquire(ctx)
    if err != nil {
        return err
    }
    defer release()

    j, err := n4UnifiedRead(path)
    if errors.Is(err, os.ErrNotExist) {
        return nil // idempotent after an independently confirmed prior cleanup
    }
    if err != nil {
        return err
    }
    if j.Identity != id || j.Phase != "applying" || len(j.Records) != 1 ||
        j.Records[0].Name != "guard" || j.Records[0].State != "owned" ||
        string(j.Records[0].Before) != `{"absent":true}` {
        return ErrN4JournalConflict
    }
    // Presence of a separate route WAL (even corrupt) makes the state
    // ambiguous. Do not follow symlinks or attempt to delete that sidecar.
    if _, err := os.Lstat(path + ".routes"); err == nil {
        return ErrN4JournalConflict
    } else if !errors.Is(err, os.ErrNotExist) {
        return err
    }
    if err := n4OriginalWintunAbsent(ctx, id); err != nil {
        return err
    }
    state, err := g.ownedState(ctx) // exact sublayer/rule CAS; foreign fails
    if err != nil {
        return err
    }
    switch state {
    case N4UnifiedOwned, N4UnifiedPartial:
        // A crash may have interrupted this exact-owned deletion itself.
        // Missing owned rule IDs are allowed only in this guard-only
        // journal state, never while any capture-route stage could exist.
        if err := g.DisarmAfterRoutesRemoved(ctx); err != nil {
            return err
        }
    case N4UnifiedAbsent:
        // Another cleanup already removed the exact-owned policy.
    default:
        return ErrN4NativeGuard
    }
    if err := ctx.Err(); err != nil {
        return err
    }
    state, err = g.ownedState(ctx)
    if err != nil {
        return err
    }
    if state != N4UnifiedAbsent {
        return ErrN4NativeGuard
    }
    // Refuse to finalize a journal if an interface was revived/reused
    // during the WFP cleanup window. No foreign NIC is ever modified.
    if err := n4OriginalWintunAbsent(ctx, id); err != nil {
        return err
    }
    return os.Remove(path)
}
