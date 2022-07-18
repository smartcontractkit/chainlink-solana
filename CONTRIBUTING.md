🎈 Thanks for your help improving the project! We are so happy to have you!

## Workflow for Pull Requests

🚨 Before making any non-trivial change, please first open an issue describing the change to solicit feedback and guidance. This will increase the likelihood of the PR getting merged.

In general, the smaller the diff the easier it will be for us to review quickly.

In order to contribute, fork the appropriate branch, for non-breaking changes to production that is `develop` and for the next release that is normally `release/X.X.X` branch.

Additionally, if you are writing a new feature, please ensure you add appropriate test cases.

Follow the [Getting Started](./docs/getting-started.md) guide to set up your local development environment.

We recommend using the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) format on commit messages.

Unless your PR is ready for immediate review and merging, please mark it as 'draft' (or simply do not open a PR yet).

**Bonus:** Add comments to the diff under the "Files Changed" tab on the PR page to clarify any sections where you think we might have questions about the approach taken.

### Response time:

We aim to provide a meaningful response to all PRs and issues from external contributors within 5 business days.

### Changesets

We use [changesets](https://github.com/atlassian/changesets) to manage releases of our various packages.
You _must_ include a `changeset` file in your PR when making a change that would require a new package release.

Adding a `changeset` file is easy:

1. Navigate to the root of the monorepo.
2. Run `yarn changeset`. You'll be prompted to select packages to include in the changeset. Use the arrow keys to move the cursor up and down, hit the `spacebar` to select a package, and hit `enter` to confirm your selection. Select _all_ packages that require a new release as a result of your PR.
3. Once you hit `enter` you'll be prompted to decide whether your selected packages need a `major`, `minor`, or `patch` release. We follow the [Semantic Versioning](https://semver.org/) scheme. Please avoid using `major` releases for any packages that are still in version `0.y.z`.
4. Commit your changeset and push it into your PR. The changeset bot will notice your changeset file and leave a little comment to this effect on GitHub.

### Rebasing

We use the `git rebase` command to keep our commit history tidy.
Rebasing is an easy way to make sure that each PR includes a series of clean commits with descriptive commit messages
See [this tutorial](https://docs.gitlab.com/ee/topics/git/git_rebase.html) for a detailed explanation of `git rebase` and how you should use it to maintain a clean commit history.
