# P5 soak workflow quoting repair

- Date: 2026-09-21
- Affected commit: `e92337d6125288445b136f6697b1be8c394fad41`
- Actions: `35587991602`
- Result: workflow-level immediate failure, zero jobs

Raw content from the actual commit showed the soak shell block was still corrupted around the anchored `-run='^...$'` command.

The replacement now avoids that quoting/transport edge entirely and uses one shell line:

```
go test ./internal/runtimeentry -count=1 -timeout=40m -run=TestP5ThirtyMinuteSoakMeasurementHarness -v | tee "$WBD_P5_SOAK_ARTIFACT_DIR/test.log"
```

The test name is unique, so removing the regex anchors does not broaden the intended soak gate. The new blob was read back before commit and contained the complete command.

No product, soak scenario, validator, or acceptance thresholds change.
