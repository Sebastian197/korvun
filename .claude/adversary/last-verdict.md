VETO LEVANTADO c3a6d997631a3387f1b3df50badd3b5149d64b34

Round: the marker master lost to the rebase button.

WHY THIS COMMIT EXISTS. The approvals-screen train (#36) carried a proper
marker commit naming its code commit, `ef1f7c2`. The pull request was
merged with REBASE, and a GitHub rebase writes new commits: the code
commit landed on master as `6843883`, so the marker inside `c3a6d99`
names a SHA that is not its parent in this history. The push gate said
so plainly and refused:

    the marker at c3a6d997… records ef1f7c2a…, but the judged commit's
    parent is 6843883660c8… — the recording commit does not name its
    direct parent.

This commit changes exactly one path and its first line names its direct
parent, `c3a6d99`.

WHY THE LEGACY FORM AND NOT THE VERSIONED ONE. The versioned evidence
(`KORVUN-REBASE-EVIDENCE v1`) validates a marker that TRAVELLED INSIDE
the pull request that integrated it: it requires that pull request's
merge result to be the judged commit itself. A repair written after the
merge is always a different commit, so the checker refuses it. Captured
against master's own gate, on a candidate built over `c3a6d99`:

    REBASE_PROVENANCE: PR does not bind source and integrated history

The legacy form is the one the gate accepts here (exit 0, captured the
same way). Making the versioned form pass for a post-merge repair is
filed; it is not done here.

WHAT IT ATTESTS, and what it does not. It attests that `c3a6d99` — the
tip carrying #36 — is the commit judged. It does NOT re-open that train's
review: its own marker, recording that the veto was lifted by the
director and not by the adversary, is preserved in the history of
`c3a6d99` and in the source branch of #36. Nothing is claimed here that
was not claimed there.

STATE: the tag gate is the only thing this commit unblocks. Not
VERIFIED, not ACCEPTED: those words belong to the gate and to the
director.
