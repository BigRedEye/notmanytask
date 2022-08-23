#!/bin/bash

set -ex

YC_REGISTRY_ID=$1
GITLAB_REPO_RUNNER_TOKEN=$2
GITLAB_GROUP_RUNNER_TOKEN=$3

. setup-common.sh

sudo su root -c 'cat sa-grader-key.json | docker login --username json_key --password-stdin cr.yandex'

sudo gitlab-runner register \
  --non-interactive \
  --url "https://gitlab.com/" \
  --registration-token "$GITLAB_REPO_RUNNER_TOKEN" \
  --executor "docker" \
  --docker-image "cr.yandex/$YC_REGISTRY_ID/hse-cxx-build:latest" \
  --tag-list ""

sudo gitlab-runner register \
  --non-interactive \
  --url "https://gitlab.com/" \
  --registration-token "$GITLAB_GROUP_RUNNER_TOKEN" \
  --executor "docker" \
  --docker-image "cr.yandex/$YC_REGISTRY_ID/hse-cxx-testenv:latest" \
  --tag-list ""

rm sa-grader-key.json
