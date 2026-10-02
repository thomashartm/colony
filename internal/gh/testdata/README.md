# gh fixtures

- `issue-12.json`: recorded 2026-10-02 with gh 2.83.2 from thomashartm/motley#12
  using the `gh api graphql` query in `issues.go`.
- `issue-parent.json`: `issue-12.json` with `parent` and `milestone` filled in by
  hand; #12 has neither, so no real parent exists to record.
- `prs.json`: recorded 2026-10-02 with gh 2.83.2 by
  `gh pr list --repo thomashartm/motley --state all --limit 3 --json number,url,state,isDraft,reviewDecision,statusCheckRollup,headRefName`
  (#42–#44, merged, every check run completed).
