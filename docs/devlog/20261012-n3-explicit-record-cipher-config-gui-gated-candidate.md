# 2026-10-12 N3-D: AES record cipher config in Linux/Windows client, gated GUI Transport control

Prior exact N3 TLS authenticated admission SOURCE 16517ff09a980391e083b4144bcaa9133087e25e, Action38267871609 job114874521000 artifact11755447591 SHA256 da34f0f056071c024af07df147d588feef739569ec13cf1b986a61eb09875dbf: unit/race/vet cross-build PASS; REAL AES network NOT_RUN.

This candidate implements ParseRecordCipherName with exactly chacha20-poly1305(default), aes-128-gcm, aes-256-gcm; rejects empty, mixed-case, unknown and outer TLS suite names. Both clients set --record-cipher and use PolicyForClientWithInitialAndCipher to put the suite in protected V3 admission. Strict JSON configfile uses the same flag. Existing legacy/empty FEC mode rejects explicit AES to avoid implicit legacy migration. Outer TLS camouflage is independent of this authenticated data record cipher.

Windows GUI uses existing Transport section: FEC mode/minimum/start, authenticated live quality label and new record-cipher field. Before actual AES128/256 five-netns tests, GUI only lists ChaCha and warns that AES network/resource qualification is pending. No false AES GUI advertising. After individually verified real AES Actions, enable the AES ComboBox choices under Transport with native configuration/screenshot proof.

New GitHub config workflow enforces source, Go policy/parser/real admission, full unit/race/vet, Linux client + Windows client + Linux arm64 server cross-build, no performance job. Existing native WinForms GUI workflow independently validates strict source catalog and displayed page. Do not preclaim either Action outcome.

Existing N2 normal auto has only scoped noHOL and native GUI checks; C04 S2C recover5 78.3% and C05 S2C10% 70% efficiency FAIL and C10 GE p99 1.8-1.95s OPEN. N3 AES real protected traffic and matched CPU baseline NOT_RUN, N4 TUN/N5 complete GUI/N6 packages open. physical NOT_RUN, do not deploy/merge/advance qualification ref.
