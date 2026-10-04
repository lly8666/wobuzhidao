# Preserve functional configuration override after DNS opt-out

Parent4da2808; no product/runtime change. The historical port53 echo fixture explicitly disables new DNS hijack. Its existing argument preparer assumed every original CLI flag had a separate value; the new --dns-hijack=false single token shifted pair parsing and could leave old MTU CLI flags overriding JSON. Handle equals-form flags as single tokens; retain unrelated flags, remove targeted override flags correctly. Added JSON-MTU regression with opt-out preceding ordinary flags.

All execution in Actions. This fixes fixture correctness; no relaxed DNS, MTU, performance or loss gates. Default DNS independently tested in next-default-network, echo configs explicitly off. Freeze this source for core/race + independent Normal/Game5205 + separate profile + lifecycle/P6. Preserve prior mock/profile failures. Full70/18/1800s not inherited.
