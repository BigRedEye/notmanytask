resource "yandex_compute_instance_group" "graders" {
  name                = "graders"
  service_account_id  = "${yandex_iam_service_account.sa-ig-editor.id}" 

  instance_template {
    resources {
      memory = 8
      cores  = 4
    }
    boot_disk {
      initialize_params {
        image_id = var.image-id
        size     = 100
      }
    }
    network_interface {
      subnet_ids = ["${yandex_vpc_subnet.subnet-hse.id}"]
    }
    metadata = {
      user-data = file("${path.module}/cloud_config.yaml")
    }
    service_account_id = "${yandex_iam_service_account.sa-grader.id}"
  }

  scale_policy {
    fixed_scale {
      size = 2
    }
  }

  allocation_policy {
    zones = ["${var.zone}"]
  }

  deploy_policy {
    max_unavailable = 2
    max_expansion   = 2
  }

  depends_on = [
    yandex_resourcemanager_folder_iam_member.editor,
  ]
}

resource "yandex_compute_instance" "builder" {
  name = "builder"
  service_account_id = "${yandex_iam_service_account.sa-builder.id}"

  resources {
    cores = 4
    memory = 8
  }

  boot_disk {
    initialize_params {
      image_id = var.image-id
      size     = 100
    }
  }

  network_interface {
    subnet_id = "${yandex_vpc_subnet.subnet-hse.id}"
  }

  metadata = {
    user-data = file("${path.module}/cloud_config.yaml")
  }
}
