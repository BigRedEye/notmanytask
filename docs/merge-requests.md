# Merge request workflow

Enabled by the `gitlab.mergeRequests` block in the config; without it the
server scores pipelines on `submits/<task>` branches as before.

## Flow

1. A student's repository is a fork of `gitlab.templateProject` into the
   course group. The student pushes a solution to `submits/<task>`.
2. Within `pullIntervals.mergeRequests` the robot opens a merge request from
   that branch into `targetBranch`. CI runs on it, reviewers leave threads.
3. When the request is quiet (last pipeline and last review thread older
   than `reviewTtl`), green, mergeable, has no unresolved threads and only
   touches `tasks/<task>/`, the robot merges it. `reviewTtl: 0` keeps
   requests tracked and scored but never merges.
4. A merged request scores like a successful pipeline submitted at its last
   pipeline time, through the deadline policy of the group.

## Re-checks

Merged requests keep being synced. To re-check a task with new tests, run a
pipeline on every `submits/<task>` branch (the merged head): a red result
takes the credit away and the task shows as failed. A push past the merged
head gets a new merge request, which goes through the same flow; the earliest
green merged request of the task wins, so a later merge never lowers a score.

## Task status

| merge request | status | score |
|---|---|---|
| merged, last pipeline green | `success` | by pipeline time |
| merged, last pipeline not green (re-check) | `failed` | 0 |
| open, last pipeline failed | `failed` | 0 |
| open, touches files outside `tasks/<task>/` | `failed` | 0 |
| open, gitlab says conflict | `failed` | 0 |
| open, unresolved review threads | `review_unresolved` | 0 |
| open, threads resolved, waiting | `review_resolved` | 0 |
| open, green, waiting for the review period | `review_pending` | 0 |
| closed | `review_pending` | 0 |
| no merge request | `assigned` | 0 |

The status comes from the request that represents the task: the highest row
above among the task's requests; among merged ones, a creditable request with
the earliest pipeline. Flags (crashme) and overrides apply on top, as in the
pipeline workflow. A review mark is shown when any request of the task has
notes or was merged by a person rather than the robot.
