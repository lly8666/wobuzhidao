# Windows lane automatic rotation policy

`Profile.LaneRotationMinSeconds` and `Profile.LaneRotationMaxSeconds` control scheduled per-lane age replacement.

- `min=0` and `max=0`: automatic age-based lane rotation is disabled. Existing scheduled age deadlines are cleared and no automatic `ReplaceLane` is issued.
- Both values must be positive to enable automatic rotation. Supplying exactly one zero is invalid.
- The minimum enabled value is 10 seconds.
- `max` must be greater than or equal to `min`.
- `min=max` requests a fixed automatic rotation interval; for example `90/90` rotates lanes on the 90-second policy used by soak tests.
- Manual lane replacement remains available when automatic rotation is disabled; the `0/0` contract only disables the age scheduler.

The historical product constants `DefaultLaneRotationMinSeconds` (30 minutes) and `DefaultLaneRotationMaxSeconds` (60 minutes) remain available to callers that explicitly want those values. They are no longer implicitly substituted for a zero-valued profile.
