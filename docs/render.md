# Publishing a course: `nmt render`

The private course repository holds everything: statements, public and
private tests, reference solutions, deadlines. `nmt render` builds the public
tree students fork from, and git publishes it.

## Layout

```
course.yaml
deadlines/ami.yml
tasks/palindrome/                    statement, stubs, public tests: exported
tasks/future/                        not in any deadlines file: private
private/palindrome/                  private tests: never exported
private/palindrome/solution/         reference solution: never exported
testenv.docker                       builds the grader image from this repository
```

A task is exported iff it is listed in one of the deadlines files. Adding a
task to the deadlines is what releases it. Directories named `private` or
`solution` are never exported, anywhere in the tree.

Private tests and the reference solutions do not go to students at all: the
grader image is built from the private repository (`COPY . /opt/shad`, then
`solution` directories are removed) and the student's CI runs the grader
inside that image. The layout above is what the grader expects
(`private/<task>/`), so keep it.

## course.yaml

```yaml
tasks: tasks                       # directory with one subdirectory per task
deadlines: [deadlines/ami.yml]     # files that define the public tasks
deadlinesFormat: v2
export:
  include:                         # files outside the tasks, gitignore-like globs
    - "cmake/**"
    - "CMakeLists.txt"
    - "*.md"
    - ".gitlab-ci.yml"
    - "deadlines/**"
  exclude: []                      # never exported, on top of private/solution
  forbid: ["Private_"]             # substrings that must not appear in any exported file
```

## Running

```sh
nmt render --source . --out ./public
```

`--out` is made to match the public tree exactly: files are written, stale
ones deleted, a `.git` directory inside is left alone. The command prints the
changed files and refuses to write anything if a forbidden substring is found.

## Publishing

```sh
nmt publish --source . --target git@gitlab.example.org:course/template.git [--dry-run]
```

Clones the template, renders into the clone and pushes one commit
`Publish <date> <time> from <short hash of the source>`; nothing is pushed when the public tree did not change.
`--dry-run` shows the diff instead. Authentication is git's own: an ssh key,
or a token in the URL. The template is never force-pushed: student forks
update from it.

## Installing nmt

- On your machine: download `nmt-<os>-<arch>` from the [releases](https://github.com/BigRedEye/notmanytask/releases) (check it against `sha256sums.txt`), or `go install github.com/bigredeye/notmanytask/cmd/nmt@latest`.
- In CI: the `ghcr.io/bigredeye/notmanytask-cli:<version>` image has `nmt` and `git` and no entrypoint, so any CI runs its scripts as is. Pin the version: rendering rules change with notmanytask.

## Publishing from CI

Two projects on the course GitLab: a private **beta** that follows `main` of the course repository automatically, and the **public** template students fork from, updated by a button. The button copies the head of beta as is, so what goes to students is exactly what you looked at in beta. Nothing is ever force-pushed: pressing the button in an old pipeline still ships the current beta, and a commit made in public by hand makes the push fail instead of being overwritten.

```yaml
variables:
  BETA_URL: https://token:${BETA_TOKEN}@gitlab.example.org/course/beta.git
  PUBLIC_URL: https://token:${PUBLIC_TOKEN}@gitlab.example.org/course/public.git

.nmt:
  image: ghcr.io/bigredeye/notmanytask-cli:v1.0.0
  stage: publish
  variables:
    GIT_SUBMODULE_STRATEGY: none    # private tests are not needed to publish

beta:                               # every push to main
  extends: .nmt
  needs: [testenv]                  # the grader image with the new tasks goes first
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  script:
    - nmt publish --source . --target "$BETA_URL"

public:                             # the button
  extends: .nmt
  needs: [beta]
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual
      allow_failure: true           # otherwise every main pipeline shows as blocked
  script:
    - git fetch "$BETA_URL" main
    - git push "$PUBLIC_URL" FETCH_HEAD:main
```

`BETA_TOKEN` and `PUBLIC_TOKEN` are project access tokens of the two projects (Maintainer, `write_repository`), stored as masked and protected CI variables of the course repository, so branch pipelines cannot read them. Review what the button will ship in beta: its commits after the one public already has.
