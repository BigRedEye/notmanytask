resource "yandex_container_registry" "cr-hse" {
  name      = "cr-hse"
}

resource "yandex_container_registry_iam_binding" "puller" {
  registry_id = "${yandex_container_registry.cr-hse.id}"
  role        = "container-registry.images.puller"

  members = [
    "serviceAccount:${yandex_iam_service_account.sa-grader.id}",
    "serviceAccount:${yandex_iam_service_account.sa-builder.id}",
  ]
}

resource "yandex_container_registry_iam_binding" "pusher" {
  registry_id = "${yandex_container_registry.cr-hse.id}"
  role        = "container-registry.images.pusher"

  members = [
    "serviceAccount:${yandex_iam_service_account.sa-builder.id}",
  ]
}
