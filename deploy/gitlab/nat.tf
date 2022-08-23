
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
      image_id = "fd80mrhj8fl2oe87o4e1"
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
