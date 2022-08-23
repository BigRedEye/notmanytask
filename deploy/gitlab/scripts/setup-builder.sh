#!/bin/bash

set -ex

GITLAB_REPO_RUNNER_TOKEN=$1

. setup-common.sh

sudo su gitlab-runner -c 'cat sa-builder-key.json | docker login --username json_key --password-stdin cr.yandex'

sudo gitlab-runner register \
  --non-interactive \
  --url "https://gitlab.com/" \
  --registration-token "$GITLAB_REPO_RUNNER_TOKEN" \
  --executor "shell" \
  --tag-list "docker"

rm sa-builder-key.json
