# Порядок настройки

1) Настроить облако и каталог в YC CLI через `yc init`.

2) Установить `YC_OAUTH`, `YC_CLOUD_ID`, `YC_FOLDER_ID`, далее сгенерировать `vars.tf` командой
```
envsubst < vars.tf.example > vars.tf
```

А также прописать нужные ssh-ключи в cloud_config.yaml.

3) Поднять облако через terraform
```
terraform init
terraform apply
```

Если все поднимется, в output будет `YC_REGISTRY_ID` и внешний IP для ssh-jump.

Внутренние IP можно получить с помощью `yc compute instance-group list-instances --id INSTANCE_GROUP_ID`, а `INSTANCE_GROUP_ID` можно найти в output terraform. 

4) Сгенерировать ключи для сервисных аккаунтов
```
yc iam key create --service-account-name sa-builder -o sa-builder-key.json
yc iam key create --service-account-name sa-grader -o sa-grader-key.json
```
5) Пройтись по раннерам и настроить их (пока не автоматизировано)

* Настройка билдера (ожидается, что ключ `sa-builder-key.json` лежит в текущем каталоге)
```
./setup-builder.sh GITLAB_REPO_RUNNER_TOKEN
```
* Настройка грейдеров (ожидается, что ключ `sa-grader-key.json` лежит в текущем каталоге)

```
./setup-grader.sh YC_REGISTRY_ID GITLAB_REPO_RUNNER_TOKEN GITLAB_GROUP_RUNNER_TOKEN
```
  
6) Собрать локально базовый образ и запушить его в созданный registry (из корня репозитория)
```
docker build -f build.docker -t cr.yandex/$YC_REGISTRY_ID/hse-cxx-build:latest .
docker push cr.yandex/$YC_REGISTRY_ID/hse-cxx-build:latest
```
7) Закоммитить `$YC_REGISTRY_ID` в репозиторий курса (в `.gitlab-ci.yml`, `.grader-ci.yml`, `testenv.docker`). CI должен пройти. Сдача задач студентами также должна работать, им для этого нужно сделать pull изменений.

8) Настроить автоматическое удаление образов.
```
echo '[{"description": "Delete stale images", "tag_regexp": ".*", "retained_top": 2}]' > rules.json

yc container repository lifecycle-policy create \
    --repository-name $YC_REGISTRY_ID/hse-cxx-build  \
    --name auto_cleanup \
    --description "" \
    --rules rules.json \
    --active

yc container repository lifecycle-policy create \
    --repository-name $YC_REGISTRY_ID/hse-cxx-testenv  \
    --name auto_cleanup \
    --description "" \
    --rules rules.json \
    --active
```

То есть будут удаляться все образы, кроме двух последних.
