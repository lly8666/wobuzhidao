# Final portable defaults and crash recovery fencing

Parent0bcfe99 Windows GUI run37185218018 PASS including real GUI manual China list update and1500route Apply/Cleanup/rollback ownership mock. Previously sourcecc204checks+visual PASS. No performance workloads changed.

Review of portability found bare packaged CLI still defaulted state to callerCWD and network script to source-tree scripts/ although package script at root. Change Windows CLI defaults to executable-directory data/network-state.json and root windows_client_network.ps1. PARAMETERS regenerated with allowed generator, docs updated. GUI managed fields never need to parse these filesystem default expressions; ordinary setting defaults continue authoritative catalog. New Windows unit compares default paths to actual os.Executable.

Crash GUI may leave its Go child cleaning after GUImutex disappears. Existing named event fences new child start. Add read-only same-event detection for GUI recovery command; reject if any live client holds it, so recovery cannot race old cleanup. When old process really ended and owned journal persists, next connect runs state-only recovery before starting new client. Fake recovery-busy test added; no extra network architecture or secrets in command line.

Last source candidate needs exact GUI/package/core proof and screenshot. No more optimization or parameter sweep; all local changes read/edit/generator only, all test/build on Actions. UserWintun approval retained. Physical/UAC/Npcap/NIC P7 NOT_RUN, historical full70/18/1800 not inherited.