# Protecting `main`

Branch protection is configured in GitHub, not by files in this repository.
The `main` branch currently has these protections:

- Require pull requests before merging (no approval count required)
- Require the `checks` status check from the `CI` workflow and require the
  branch to be up to date
- Block force pushes
- Block branch deletion
- Enforce the rules for administrators

To reproduce these protections in another repository, create an active branch
ruleset in **Settings → Rules → Rulesets** targeting `main`. The `checks` job is
provided by [`.github/workflows/ci.yml`](workflows/ci.yml).
