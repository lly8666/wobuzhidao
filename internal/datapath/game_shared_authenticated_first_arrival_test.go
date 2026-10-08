package datapath

import (
	"bytes"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

)

// A Game "shared pot" already exists AFTER authenticated per-lane decode.
// With 2 or 4 concurrently arriving lanes, the logical PacketID must be
// committed exactly once; arriving order is deliberately unconstrained.
func TestGameSharedAuthenticatedFirstArrivalTwoAndFourLanes(t *testing.T) {
	for _, lanes := range []int{2, 4} {
		t.Run(fmt.Sprintf("lanes-%d", lanes), func(t *testing.T) {
			lease := gameLease(t)
			src, err := lease.Config.LeaseIPv4()
			if err != nil { t.Fatal(err) }
			pair := newGameOwnerPair(t, lease, lanes)
			now := time.Unix(1970, 0)
			packet := businessIPv4Packet(src, netip.MustParseAddr("203.0.113.10"), []byte("authenticated-first-arrival"))
			out, err := pair.client.GameOutbound(packet, now)
			if err != nil || len(out.Lanes) != lanes {
				t.Fatalf("outbound lanes=%d err=%v", len(out.Lanes), err)
			}
			type answer struct {
				datagrams [][]byte
				err error
			}
			results := make(chan answer, lanes)
			var wg sync.WaitGroup
			for _, item := range out.Lanes {
				item := item
				wg.Add(1)
				go func() {
					defer wg.Done()
					var received [][]byte
					for _, record := range item.Records {
						var result InboundResult
						var err error
						result, err = pair.server.GameInboundPayload(item.Ref, record.Wire, now)
						if err != nil { results <- answer{err:err};return }
						if len(result.RecordErrors)!=0 || len(result.PathErrors)!=0 {
							results <- answer{err:fmt.Errorf("record=%v path=%v",result.RecordErrors,result.PathErrors)}
							return
						}
						received = append(received, result.Datagrams...)
					}
					results <- answer{datagrams:received}
				}()
			}
			wg.Wait()
			close(results)
			delivered := 0
			for v := range results {
				if v.err != nil { t.Fatal(v.err) }
				for _, got := range v.datagrams {
					if !bytes.Equal(got, packet) { t.Fatal("wrong authenticated payload") }
					delivered++
				}
			}
			if delivered != 1 {
				t.Fatalf("%d Game lanes delivered %d copies, want exactly one", lanes, delivered)
			}
			diag := pair.server.Stats()
			if diag.GameDelivered != 1 || diag.GameDuplicates != uint64(lanes-1) {
				t.Fatalf("shared dedupe stats: %+v", diag)
			}
			// The owner already checks ref generation / lease source before
			// entering shared PacketID dedupe; this concurrency does not
			// create a new unauthenticated admission path.
		})
	}
}
