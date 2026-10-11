package datapath

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func n2AEADDeliver(t *testing.T,dst *Lane,records []WireRecord,at time.Time) [][]byte {
	t.Helper()
	var got [][]byte
	for _,r:=range records {
		result,err:=dst.InboundPayload(r.Wire,at)
		if err!=nil||len(result.RecordErrors)!=0||len(result.PathErrors)!=0 {
			t.Fatalf("AEAD/link inbound failed err=%v rec=%v path=%v",err,result.RecordErrors,result.PathErrors)
		}
		got=append(got,result.Datagrams...)
	}
	return got
}

func TestN2ActualAEADOffOnNewProfileAndOldDelayedFirstDelivery(t *testing.T) {
	clientConfig:=pairConfig(RoleClient,20)
	serverConfig:=pairConfig(RoleServer,20)
	// pairConfig intentionally uses 8ms for older codec tests. This N2
	// production-default lifetime test explicitly promises 32ms.
	clientConfig.FlushAfter=32*time.Millisecond
	serverConfig.FlushAfter=32*time.Millisecond
	client,err:=NewN2AdaptiveLaneForTests(clientConfig)
	if err!=nil {t.Fatal(err)}
	defer client.Close()
	server,err:=NewN2AdaptiveLaneForTests(serverConfig)
	if err!=nil {t.Fatal(err)}
	defer server.Close()
	t0:=time.Unix(10000,0)
	mtu:=client.TxBudget()
	old,err:=client.Outbound([]byte("old-systematic-lost"),t0)
	if err!=nil||len(old)!=1||!old[0].FEC.Enabled {t.Fatalf("initial FEC source %d err=%v",len(old),err)}
	if err:=client.SwitchN2ParityForTests(0);err!=nil {t.Fatal(err)}
	fresh,err:=client.Outbound([]byte("fresh-off"),t0.Add(time.Millisecond))
	if err!=nil||len(fresh)!=1||fresh[0].FEC.Enabled {t.Fatalf("off new source %d err=%v",len(fresh),err)}
	if got:=n2AEADDeliver(t,server,fresh,t0.Add(time.Millisecond));len(got)!=1||string(got[0])!="fresh-off" {
		t.Fatalf("fresh off blocked by old partial: %q",got)
	}
	// A legal bare packet with the FEC header prefix is escaped BEFORE the
	// encrypted record, and still first-delivers independently of old parity.
	wf:=[]byte("WF-this-is-a-legitimate-bare-payload")
	wire,err:=client.Outbound(wf,t0.Add(2*time.Millisecond))
	if err!=nil||len(wire)!=1||wire[0].FEC.Enabled {t.Fatalf("WF off wrong meta %v",err)}
	if got:=n2AEADDeliver(t,server,wire,t0.Add(2*time.Millisecond));len(got)!=1||!bytes.Equal(got[0],wf) {
		t.Fatalf("WF collision lost data %q",got)
	}
	if err:=client.SwitchN2ParityForTests(8);err!=nil {t.Fatal(err)}
	other,err:=client.Outbound([]byte("new-systematic-8"),t0.Add(3*time.Millisecond))
	if err!=nil||len(other)!=1||!other[0].FEC.Enabled||other[0].FEC.ConfiguredParity!=8 {
		t.Fatalf("new source wrong live geometry %d %v",len(other),err)
	}
	if got:=n2AEADDeliver(t,server,other,t0.Add(3*time.Millisecond));len(got)!=1||string(got[0])!="new-systematic-8" {
		t.Fatalf("new FEC systematic waited for ACK/old parity %q",got)
	}
	if early,err:=client.FlushDue(t0.Add(31*time.Millisecond));err!=nil||len(early)!=0 {
		t.Fatalf("switch accelerated old deadline %d %v",len(early),err)
	}
	late,err:=client.FlushDue(t0.Add(32*time.Millisecond))
	if err!=nil||len(late)!=1||!late[0].FEC.Enabled||late[0].FEC.ConfiguredParity!=20||
		late[0].FEC.ClosedK!=1 {t.Fatalf("old parity changed when new 20:8 active %#v %v",late,err)}
	if got:=n2AEADDeliver(t,server,late,t0.Add(32*time.Millisecond));len(got)!=1||string(got[0])!="old-systematic-lost" {
		t.Fatalf("late parity did not recover original first delivery %q",got)
	}
	if got:=n2AEADDeliver(t,server,old,t0.Add(50*time.Millisecond));len(got)!=0 {
		t.Fatalf("late original delivered twice %q",got)
	}
	if client.TxBudget()!=mtu {t.Fatalf("TUN/path MTU changed across auto mode switch %v -> %v",mtu,client.TxBudget())}
	if stats:=client.Stats();!stats.TxPath.Adaptive||stats.TxPath.DrainingEncoders!=0||
		stats.TxPath.Encoder.ClosedByK[1]==0 {t.Fatalf("missing source/parity closing evidence %+v",stats.TxPath)}
}

func TestN2ExplicitLaneOptInCannotMutateLegacyFixed(t *testing.T) {
	fixed,peer:=lanePair(t,20)
	defer fixed.Close()
	defer peer.Close()
	if err:=fixed.SwitchN2ParityForTests(0);!errors.Is(err,linkdata.ErrAdaptiveSwitchUnavailable) {
		t.Fatalf("fixed silently migrated %v",err)
	}
	if _,err:=NewN2AdaptiveLaneForTests(pairConfig(RoleClient,4));!errors.Is(err,ErrConfigMismatch) {
		t.Fatalf("adaptive max FEC MTU must be reserved upfront %v",err)
	}
	at:=time.Unix(11000,0)
	wire,err:=fixed.Outbound([]byte("still-fixed"),at)
	if err!=nil||len(wire)!=1||!wire[0].FEC.Enabled||wire[0].FEC.ConfiguredParity!=20 {
		t.Fatalf("legacy wire changed %d %v",len(wire),err)
	}
	if got:=n2AEADDeliver(t,peer,wire,at);len(got)!=1||string(got[0])!="still-fixed" {
		t.Fatalf("fixed business delivery changed %q",got)
	}
}
