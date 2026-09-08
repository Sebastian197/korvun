VETO LEVANTADO 7c219028755b7f0c33407d570ade7f78dc034c0e

Round: the marker master lost to the squash button.

WHY THIS COMMIT EXISTS. The R15 and hygiene trains each carried a proper
marker commit, and both pull requests were merged with SQUASH. A squash
collapses the marker and the code into ONE commit, so master's tip
changed eleven files — production code included — and the marker text
inside it still named `2b0edc8`, the pre-squash SHA that no longer
exists in this history. The push gate said so plainly and refused:

    the marker at 7c21902 records 2b0edc8, but the judged commit's
    parent is f3d509f — the recording commit does not name its direct
    parent

This commit is the marker master should have had: it changes exactly one
path and its first line names its direct parent.

WHAT IT ATTESTS, and what it does not. It attests that `7c21902` — the
tip carrying the hygiene batch (#29) and the R15 train (#30) — is the
commit judged. It does NOT re-open either train's review: both were
delivered under the director's order of 2026-09-08, each with its own
marker recording that the veto was lifted by HIM and not by the
adversary, and each of those markers is preserved in the history of its
pull request. Nothing is claimed here that was not claimed there.

A NOTE FOR THE NEXT TRAIN, because this cost a release day. A marker
commit and a squash merge are incompatible by construction: the marker's
whole contract is "I name my direct parent", and squash rewrites the
parent after the marker is written. Either the merge preserves the two
commits, or the marker is created after the merge — as this one is. That
is a process decision for the director, and it is written here rather
than left to be rediscovered.

STATE: the tag gate is the only thing this commit unblocks. Not
VERIFIED, not ACCEPTED: those words belong to the gate and to the
director.
