# 20260920-181000 P4 OpenWrt socket harness parse fix

## Failed exact-SHA evidence

SOURCE_SHA: 64b54310215898fbc9a2a559bbd7104acea9dcda
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35496266975
Result: completed / failure.

PASS: repository-contract; Windows/Linux unit/build; Linux race/fuzz/reference; P2 kernel fallback; Linux privileged shared-TUN iptables+nft.

The OpenWrt privileged job also compiled ./internal/openwrtclient successfully. It failed before either privileged Go test ran:

scripts/openwrt_tproxy_netns.sh: line 73: unexpected EOF while looking for matching single quote

Artifact: 10601440103
Digest: sha256:7536911fcaf09c86955658673885f8dbe8f75248a9bcd004b0a5980287c9cb79

## Root cause

The previous programmatic harness edit truncated the final -test.run shell argument, leaving its opening single quote without a closing regex suffix/quote. The same failed text replacement left the target namespace with only 10.20.0.2 and .3, while the new EIM/EIF fixture requires .4 and .5.

Because shell parsing failed, neither TestPrivilegedOpenWrtTPROXYRuntime nor TestPrivilegedOpenWrtSocketTunnelAdapter executed. This run has no root-netns product behavior verdict.

## Fix

Only scripts/openwrt_tproxy_netns.sh is corrected:
- assign target aliases 10.20.0.2/24, .3/24, .4/24, .5/24;
- preserve the existing client/router/target topology;
- invoke both tests with the fully closed anchored regex ^TestPrivilegedOpenWrt(TPROXYRuntime|SocketTunnelAdapter)$.

No product Go implementation, platform-service wire, TunnelOwner API, TPROXY runtime or workflow PASS criteria changes.

## Next

Push a new exact SHA and rerun the complete next-foundation matrix. Both OpenWrt markers must appear in the same privileged job before this atom can close.
