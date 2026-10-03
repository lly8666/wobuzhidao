# Live configuration qualification harness

## 本轮目标和阶段
P5/P6 pre-delivery, starting5a14282. Move from parser-only parameters to actual formal-binary behavior. No runtime algorithm or tuning change.

## 修改与原因
Declare70 finite functional cases: 56 allFEC0/4/8/10/12/16/20 × lanes1..4 × startup-paddingoff/on with conflicting JSON and explicitCLI priority;7 JSON-only knobs;1 omitted default knobs;6 MTU1280/1400/1500 with two asymmetric record-limit directions512/768 and768/512. Each Action one case, no performance claim. Mature lifecycle netns topology reused, but production binaries built without acceptance fault tag. UDP first checkpoint must remain unpadded and deliver exact payloads. Then actual DNS, plain TCP and inner HTTPS with validated certificate and102400-byte exact response. Runtime diagnostics prove selected lanes and FEC source/parity emissions, effective lifecycle settings and padding detection/application or explicit low-headroom skip. Post-bootstrap capture checks actual IP MTU/no fragmentation and TLS-like record framing/directional negotiated bounds. CI fixture generation/argument tests and bash syntax in Actions; original lifecycle sample unmodified.

## 复用来源
Current scripts/lifecycle_acceptance_sample.sh, realpath_udp_duplex.py, diagnostic helpers. Generated harness with template drift failure and retained source; no old modules. Existing fullstack covers idle/default health/recovery/fault transitions; this harness supplies previously missing live profile/config/HTTPS coverage.

## Actions证据
5a14282 foundation37119706270: all active Linux/Windows core/race, kernel fallback, LinuxsharedTUN iptables/nft, OpenWrtTPROXY PASS. Targeted37119706227 PASS. Single race pass cannot replace pending30-repeat diagnostic. Prior failed barrier37119536977 preserved. Initial runtimeca8175d final18 coordinator37117755700 PASS,18/18 original independent samples, retained aggregate and source identity; not inherited to these tools. New70 functional cases NOT_RUN until canaries on immutable exact SHA. No local tests/builds.

## 问题、排查与风险
Functional low load is not target-rate qualification. Snaplen256 cannot prove whole TCP checksum; full payload integrity and existing core checksum tests remain distinct. Default case omits FEC/lane/padding knobs, not bootstrap/platform settings. JSON-only refers tested knobs; required bootstrap/interface CLI still supplied. Startup padding is best effort at small record headroom, explicit skip is reported; ordinary1400/1300 case requires positive real padding. Hosted network namespace tests do not prove physical Npcap/Wintun/ARM or target website fingerprint equivalence.

## 下一项原子任务
Require foundation/tool fixtures/repeated30 race before independent Normal/Game180s and selected configuration canaries. Diagnose any failure from raw artifacts before full70 and formal1800s. P6 packages final same-source, P7 physical NOT_RUN.
