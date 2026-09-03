package windowsruntime

import (
	"errors"
	"fmt"
	"sort"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// RotateActiveLanes is the explicit local/manual reconnect surface for a
// connected Logical Tunnel. It snapshots the authoritative logical lane
// generations once, then refreshes those lanes one at a time through the same
// generation-fenced make-before-break lifecycle used by automatic triggers.
// Shared Game/TUN/routes remain alive throughout.
//
// If another trigger has already replaced a snapshotted lane before its turn,
// the stale manual request for that lane is skipped instead of rotating the
// fresh incarnation again. Any real candidate failure stops the manual sweep;
// already-promoted lanes stay promoted and the failing lane keeps healthy A.
func (c *Controller) RotateActiveLanes() error {
	refs, err := c.manualRotationRefs()
	if err != nil {
		return err
	}
	for _, ref := range refs {
		expected := ref
		if err := c.replaceLaneLifecycle(int(ref.ID), &expected, nil); err != nil {
			if errors.Is(err, errStaleLaneReplacement) {
				continue
			}
			return fmt.Errorf("manual reconnect lane %d: %w", ref.ID, err)
		}
	}
	return nil
}

func (c *Controller) manualRotationRefs() ([]logicaltunnel.LaneRef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != RuntimeConnected {
		return nil, fmt.Errorf("Windows runtime cannot manually reconnect while %s", c.state)
	}
	if c.lifecycle == nil {
		return nil, errors.New("Logical Tunnel lifecycle is unavailable")
	}
	snapshots := c.lifecycle.Snapshot()
	if len(snapshots) == 0 {
		return nil, errors.New("Logical Tunnel has no active Transport Lanes")
	}
	refs := make([]logicaltunnel.LaneRef, 0, len(snapshots))
	for _, snapshot := range snapshots {
		refs = append(refs, snapshot.Ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs, nil
}
