locals {
  name = "pact-madrid"

  common_tags = {
    managed-by  = "terraform"
    application = "pact"
    environment = "foundation"
  }
}

data "oci_core_images" "ubuntu_arm" {
  compartment_id           = var.compartment_ocid
  operating_system         = "Canonical Ubuntu"
  operating_system_version = "24.04"
  shape                    = "VM.Standard.A1.Flex"
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
}

resource "oci_core_vcn" "pact" {
  compartment_id = var.compartment_ocid
  cidr_blocks    = ["10.42.0.0/16"]
  display_name   = "${local.name}-vcn"
  dns_label      = "pact"
  freeform_tags  = local.common_tags
}

resource "oci_core_internet_gateway" "pact" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.pact.id
  display_name   = "${local.name}-internet-gateway"
  enabled        = true
  freeform_tags  = local.common_tags
}

resource "oci_core_route_table" "public" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.pact.id
  display_name   = "${local.name}-public-routes"
  freeform_tags  = local.common_tags

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.pact.id
  }
}

resource "oci_core_security_list" "public" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.pact.id
  display_name   = "${local.name}-subnet-security"
  freeform_tags  = local.common_tags

  egress_security_rules {
    protocol    = "all"
    destination = "0.0.0.0/0"
  }
}

resource "oci_core_subnet" "public" {
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.pact.id
  cidr_block                 = "10.42.10.0/24"
  display_name               = "${local.name}-public-subnet"
  dns_label                  = "edge"
  prohibit_public_ip_on_vnic = false
  route_table_id             = oci_core_route_table.public.id
  security_list_ids          = [oci_core_security_list.public.id]
  freeform_tags              = local.common_tags
}

resource "oci_core_network_security_group" "pact" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.pact.id
  display_name   = "${local.name}-nsg"
  freeform_tags  = local.common_tags
}

resource "oci_core_network_security_group_security_rule" "egress" {
  network_security_group_id = oci_core_network_security_group.pact.id
  direction                 = "EGRESS"
  protocol                  = "all"
  destination               = "0.0.0.0/0"
  destination_type          = "CIDR_BLOCK"
  description               = "Allow package downloads and outbound Pact Node traffic"
}

resource "oci_core_network_security_group_security_rule" "ssh" {
  network_security_group_id = oci_core_network_security_group.pact.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = var.ssh_source_cidr
  source_type               = "CIDR_BLOCK"
  description               = "Restricted administrative SSH"

  tcp_options {
    destination_port_range {
      min = 22
      max = 22
    }
  }
}

resource "oci_core_network_security_group_security_rule" "http" {
  count = var.public_web_enabled ? 1 : 0

  network_security_group_id = oci_core_network_security_group.pact.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = "0.0.0.0/0"
  source_type               = "CIDR_BLOCK"
  description               = "HTTP for certificate issuance and HTTPS redirect"

  tcp_options {
    destination_port_range {
      min = 80
      max = 80
    }
  }
}

resource "oci_core_network_security_group_security_rule" "https" {
  count = var.public_web_enabled ? 1 : 0

  network_security_group_id = oci_core_network_security_group.pact.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = "0.0.0.0/0"
  source_type               = "CIDR_BLOCK"
  description               = "Public Pact HTTPS endpoint once team authentication is enabled"

  tcp_options {
    destination_port_range {
      min = 443
      max = 443
    }
  }
}

resource "oci_core_instance" "pact" {
  compartment_id      = var.compartment_ocid
  availability_domain = var.availability_domain
  fault_domain        = var.fault_domain
  display_name        = "${local.name}-server"
  shape               = "VM.Standard.A1.Flex"
  freeform_tags       = local.common_tags

  shape_config {
    ocpus         = var.instance_ocpus
    memory_in_gbs = var.instance_memory_gb
  }

  availability_config {
    recovery_action = "RESTORE_INSTANCE"
  }

  create_vnic_details {
    assign_public_ip = true
    display_name     = "${local.name}-server-vnic"
    hostname_label   = "server"
    nsg_ids          = [oci_core_network_security_group.pact.id]
    subnet_id        = oci_core_subnet.public.id
  }

  metadata = {
    ssh_authorized_keys = trimspace(file(var.ssh_public_key_path))
    user_data = base64encode(templatefile("${path.module}/templates/cloud-init.yaml.tftpl", {
      ssh_source_ip      = trimsuffix(var.ssh_source_cidr, "/32")
      public_web_enabled = var.public_web_enabled
    }))
  }

  source_details {
    source_type             = "image"
    source_id               = data.oci_core_images.ubuntu_arm.images[0].id
    boot_volume_size_in_gbs = var.boot_volume_gb
  }

  agent_config {
    is_management_disabled = false
    is_monitoring_disabled = false

    plugins_config {
      desired_state = "ENABLED"
      name          = "Bastion"
    }

    plugins_config {
      desired_state = "ENABLED"
      name          = "OS Management Service Agent"
    }
  }

  instance_options {
    are_legacy_imds_endpoints_disabled = true
  }

  preserve_boot_volume = false
}

resource "oci_core_volume" "pact_data" {
  compartment_id      = var.compartment_ocid
  availability_domain = var.availability_domain
  display_name        = "${local.name}-data"
  size_in_gbs         = var.data_volume_gb
  vpus_per_gb         = 10
  freeform_tags       = local.common_tags
}

resource "oci_core_volume_attachment" "pact_data" {
  attachment_type                     = "paravirtualized"
  instance_id                         = oci_core_instance.pact.id
  volume_id                           = oci_core_volume.pact_data.id
  display_name                        = "${local.name}-data-attachment"
  is_pv_encryption_in_transit_enabled = true
}
