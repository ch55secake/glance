# Protecting `main`

Branch rules are a GitHub repository setting and cannot take effect until this
project has a GitHub repository. Once the repository exists, create a branch
ruleset in **Settings → Rules → Rulesets** with these settings:

- Target branch: `main`
- Require a pull request before merging (no approval count required for now)
- Require the `checks` status check from the `CI` workflow
- Block force pushes
- Block branch deletion
- Enforce the rules for administrators as well

The `checks` job is provided by [`.github/workflows/ci.yml`](workflows/ci.yml).
