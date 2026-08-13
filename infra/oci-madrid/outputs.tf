output "instance_id" {
  description = "Pact compute instance OCID."
  value       = oci_core_instance.pact.id
}

output "public_ip" {
  description = "Temporary public IP. Point the future Pact DNS record here."
  value       = oci_core_instance.pact.public_ip
}

output "ssh_command" {
  description = "Administrative SSH command."
  value       = "ssh -i ~/.ssh/pact_oci_ed25519 ubuntu@${oci_core_instance.pact.public_ip}"
}

output "data_volume_id" {
  description = "Block volume reserved for Pact and PostgreSQL durable data."
  value       = oci_core_volume.pact_data.id
}
