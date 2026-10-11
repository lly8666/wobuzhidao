package datapath

import (
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestN2ColdPreparedOwnerSwitchRejectsRetiredAndForeignLane(t *testing.T) {
	owner,err:=NewTunnelOwner(1,8)
	if err!=nil {t.Fatal(err)}
	defer owner.Close()
	lane,err:=NewN2AdaptiveLaneForTests(pairConfig(RoleClient,20))
	if err!=nil {t.Fatal(err)}
	snap,err:=owner.AttachInitial(1,lane)
	if err!=nil {t.Fatal(err)}
	prepared,err:=owner.PrepareN2ActiveParityForTests(snap.Ref,4)
	if err!=nil {t.Fatal(err)}
	// Cold preparation is NOT a switch and does not block/mutate fresh.
	at:=time.Unix(21000,0)
	old,err:=lane.Outbound([]byte("normal-while-other-encoder-is-cold"),at)
	if err!=nil||len(old)!=1||old[0].FEC.ConfiguredParity!=20 {
		t.Fatalf("cold allocation changed fresh original %v %d",err,len(old))
	}
	if err:=owner.CommitN2ActiveParityForTests(snap.Ref,prepared);err!=nil {t.Fatal(err)}
	if err:=owner.CommitN2ActiveParityForTests(snap.Ref,prepared);
		!errors.Is(err,linkdata.ErrAdaptiveSwitchUnavailable) {
		t.Fatalf("same cold handle committed twice: %v",err)
	}
	other,err:=owner.PrepareN2ActiveParityForTests(snap.Ref,8)
	if err!=nil {t.Fatal(err)}
	foreign,err:=NewTunnelOwner(1,8)
	if err!=nil {t.Fatal(err)}
	defer foreign.Close()
	otherLane,err:=NewN2AdaptiveLaneForTests(pairConfig(RoleClient,20))
	if err!=nil {t.Fatal(err)}
	otherSnap,err:=foreign.AttachInitial(1,otherLane)
	if err!=nil {t.Fatal(err)}
	// Even identical lane ID/generation numbers must not allow a cross-
	// client prepared encoder to attach to another owner.
	if err:=foreign.CommitN2ActiveParityForTests(otherSnap.Ref,other);err==nil {
		t.Fatal("prepared encoder crossed client owner")
	}
	if _,err:=owner.Dormant();err!=nil {t.Fatal(err)}
	if err:=owner.CommitN2ActiveParityForTests(snap.Ref,other);err==nil {
		t.Fatal("retired generation committed a cold block")
	}
}
