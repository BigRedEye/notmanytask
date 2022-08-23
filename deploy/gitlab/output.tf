output "container-registry-id" {
  value = yandex_container_registry.cr-hse.id
}

output "sa-builder-id" {
  value = yandex_iam_service_account.sa-builder.id
}

output "sa-grader-id" {
  value = yandex_iam_service_account.sa-grader.id
}

output "nat-public-ip" {
  value = yandex_compute_instance.nat-instance.network_interface.0.nat_ip_address
}

output "graders-group-id" {
  value = yandex_compute_instance_group.graders.id
}
