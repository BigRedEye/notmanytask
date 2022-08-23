resource "yandex_iam_service_account" "sa-builder" {
  name        = "sa-builder"
  description = "Able to pull and push images"
}

resource "yandex_iam_service_account" "sa-grader" {
  name        = "sa-grader"
  description = "Able to pull images"
}

resource "yandex_iam_service_account" "sa-ig-editor" {
  name        = "sa-ig-editor"
  description = "Instance group manager"
}

resource "yandex_resourcemanager_folder_iam_member" "editor" {
  folder_id = var.yandex-folder-id
  role      = "editor"
  member    = "serviceAccount:${yandex_iam_service_account.sa-ig-editor.id}"
}
