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

## Publishing from CI

```yaml
render:
  script:
    - git clone "$TEMPLATE_URL" public
    - nmt render --source . --out public
    - git -C public add -A && git -C public status --short

publish:
  when: manual
  only: [main]
  script:
    - git clone "$TEMPLATE_URL" public
    - nmt render --source . --out public
    - git -C public add -A
    - git -C public -c user.name=notmanytask -c user.email=robot@example.org commit -m "Publish $(date +%F)" || exit 0
    - git -C public push
```

`TEMPLATE_URL` is the template project with credentials for the robot, e.g.
`https://oauth2:$TOKEN@gitlab.example.org/course/template.git`. The `render`
job shows the diff on every push; `publish` is a button on `main`. Never
force-push the template: student forks update from it.
