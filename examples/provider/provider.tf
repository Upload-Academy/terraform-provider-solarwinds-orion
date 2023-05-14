terraform {
  required_providers {
    solarwinds-orion = {
      source = "upload.academy/dev/solarwinds-orion"
    }
  }
}

provider "solarwinds-orion" {
  # example configuration here
}

resource "solarwinds_orion_ip" "auto" {
    vlan_address = "10.12.72.0"
    comment = "Reserved by s1slt000321"
}

