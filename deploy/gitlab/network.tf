resource "yandex_vpc_network" "net-hse" {
  name = "net-hse"
}

resource "yandex_vpc_route_table" "nat-route-table" {
  network_id = "${yandex_vpc_network.net-hse.id}"

  static_route {
    destination_prefix = "0.0.0.0/0"
    next_hop_address = "${yandex_compute_instance.nat-instance.network_interface.0.ip_address}"
  }
}

resource "yandex_vpc_subnet" "subnet-hse" {
  name = "subnet-hse"
  zone = var.zone
  network_id = "${yandex_vpc_network.net-hse.id}"
  v4_cidr_blocks = ["10.10.10.0/24"]
  route_table_id = "${yandex_vpc_route_table.nat-route-table.id}"
}

resource "yandex_vpc_subnet" "nat-subnet-hse" {
  name = "nat-subnet-hse"
  zone = var.zone
  network_id = "${yandex_vpc_network.net-hse.id}"
  v4_cidr_blocks = ["10.10.9.0/24"]
}
