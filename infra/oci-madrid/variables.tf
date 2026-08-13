variable "tenancy_ocid" {
  description = "OCI tenancy OCID."
  type        = string
}

variable "compartment_ocid" {
  description = "Compartment in which Pact resources are created. The tenancy root is acceptable for the first deployment."
  type        = string
}

variable "region" {
  description = "OCI region for the Pact installation."
  type        = string
  default     = "eu-madrid-1"
}

variable "availability_domain" {
  description = "Availability domain name as exposed to this tenancy."
  type        = string
}

variable "fault_domain" {
  description = "Optional fault domain used to target capacity within the selected availability domain."
  type        = string
  default     = null

  validation {
    condition     = var.fault_domain == null || contains(["FAULT-DOMAIN-1", "FAULT-DOMAIN-2", "FAULT-DOMAIN-3"], var.fault_domain)
    error_message = "fault_domain must be null or one of FAULT-DOMAIN-1, FAULT-DOMAIN-2, or FAULT-DOMAIN-3."
  }
}

variable "ssh_public_key_path" {
  description = "Absolute path to the SSH public key authorized for the ubuntu user."
  type        = string
}

variable "ssh_source_cidr" {
  description = "Single trusted IPv4 CIDR allowed to reach SSH, normally the administrator's current public IP with /32."
  type        = string

  validation {
    condition     = can(cidrhost(var.ssh_source_cidr, 0)) && endswith(var.ssh_source_cidr, "/32")
    error_message = "ssh_source_cidr must be a single IPv4 /32 address."
  }
}

variable "instance_ocpus" {
  description = "Ampere A1 OCPUs allocated to Pact."
  type        = number
  default     = 1

  validation {
    condition     = var.instance_ocpus >= 1 && var.instance_ocpus <= 2
    error_message = "The first Pact deployment must use between 1 and 2 A1 OCPUs."
  }
}

variable "instance_memory_gb" {
  description = "Memory allocated to the Pact VM."
  type        = number
  default     = 4

  validation {
    condition     = var.instance_memory_gb >= 4 && var.instance_memory_gb <= 12
    error_message = "The first Pact deployment must use between 4 and 12 GB of memory."
  }
}

variable "public_web_enabled" {
  description = "Open ports 80 and 443. Keep false until Pact has a domain, TLS, and team authentication."
  type        = bool
  default     = false
}

variable "boot_volume_gb" {
  description = "Boot volume size. OCI compute images require at least 50 GB."
  type        = number
  default     = 50
}

variable "data_volume_gb" {
  description = "Separate block volume reserved for PostgreSQL and Pact durable data."
  type        = number
  default     = 50
}
