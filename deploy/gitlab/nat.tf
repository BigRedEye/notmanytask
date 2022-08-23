
resource "yandex_compute_instance" "nat-instance" {
  name = "nat-instance"

  resources {
    cores = 2
    memory = 2
    core_fraction = 5
  }

  boot_disk {
    auto_delete = true

    initialize_params {
      # NAT Instance
      # https://cloud.yandex.ru/marketplace/products/yc/nat-instance-ubuntu-18-04-lts
      image_id = "fd8q9r5va9p64uhch83k"
      size = 10
    }
  }

  network_interface {
    subnet_id = "${yandex_vpc_subnet.nat-subnet-hse.id}"
    nat = true
  }

  metadata = {
    user-data = file("${path.module}/cloud_config.yaml")
  }
}
