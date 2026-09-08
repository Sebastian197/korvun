VETO LEVANTADO 2b0edc80ed3c000ab7e21ea96250ee1e0256fc92

Round: R15 — the tombstone judged whenever it is addressable, and the
JOIN that failed open. Two changes of logic in the receipt verifier, two
corrections of message, four new molds and three executed mutations.

Rebased onto the hygiene batch on 2026-09-08, so this marker names the
code commit's SHA AFTER that rebase. The two trains both touched this
file; hygiene's marker did its job and lives in the history of master.

READ THIS BEFORE READING THE FIRST LINE AS AN AUDIT RESULT. The wire
reads only the first line, and the check script says what that line
means: "a marker whose FIRST LINE names the direct sole parent was
committed", never "audited".

**The veto was lifted by the DIRECTOR, on 2026-09-08, not by the
adversary.** Seven adversarial passes ran over this train's paper and
every one returned VETO MANTENIDO. The seventh returned five P1. One of
them — the JOIN fail-open — the director ruled INTO this train and it is
cured here, with its molds and its mutations. Everything else the pass
found is FILED with its reproduction, letreros and arithmetic included,
by his order of the same day. No eighth pass ran.

What a reader can check for himself:

- The GUARANTEE survived all seven passes untouched. What failed, every
  time, was the prose around it: the letreros, the tables, the counts,
  the citations. Each pass pushed the same defect one layer closer to the
  wire — states, mechanisms, effects, whether the mechanism was WIRED at
  all, and finally whether the row the code calls "the action row" is one
  row. That last one is why this train has a second cure.
- The fail-open was real and cheap to reach: two DELETEs of CHILD rows,
  no foreign-key override, turned exit 1 into exit 0 with two notes
  narrating a retention prune that never happened. It now fails with both
  names.
- Four molds assert the exact NAME and the exact TEXT of each outcome.
  The text half exists because this train caught itself: the wording of
  two notes was rewritten mid-train and the whole suite stayed green.
- An inherited mold was pinning a state production cannot produce — it
  deleted the action row with foreign keys OFF, leaving an orphan
  decision row the cascade never leaves. That is why it stayed green over
  the fail-open. Its fixture is now faithful; its assertions are
  unchanged.
- `make quality` green over the whole suite on the commit this marker
  names.

STATE: IMPLEMENTED. Not VERIFIED — the required checks are the gate's
word. Not ACCEPTED — that word is the director's, and the merge is his
act.
