## Summary

## Who did the work
<!-- Lead, hires and seats, reviewers (including 02's independent reviewer where applicable), and models. -->

## Links
<!-- Refinement doc path, Jira key. -->

## Checklist
- [ ] Any change to `dao.Repository` or its sub-interfaces updates the hand-written fake in this PR (decisions/test-double-strategy.md)
- [ ] Any new env var read in config.go is set in every Deployment whose APP_MODE uses it (handoff-03-auth.md v9)
- [ ] No secret or token in code, tests, or comments (gitleaks runs on this PR's commits)
- [ ] `git diff origin/main` reviewed before requesting merge
- [ ] If this PR changes `deploy/rbac.yaml`, `deploy/bootstrap.yaml`, or the `ResourceQuota`, say who applies it and when, because the pipeline does not
