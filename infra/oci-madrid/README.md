# Fundación de Pact en OCI Madrid

Esta configuración crea la base de una instalación de Pact en `eu-madrid-1`:

- VM Ampere A1 Flex con Ubuntu 24.04 ARM;
- 1 OCPU y 4 GB de RAM por defecto;
- VCN y subnet dedicadas;
- Network Security Group con SSH limitado a una sola IP;
- API y backoffice cerrados al público hasta disponer de TLS y autenticación
  de equipo;
- 50 GB de boot y 50 GB de Block Volume para datos;
- Docker, montaje persistente en `/srv/pact-data`, actualizaciones automáticas
  y endurecimiento básico del host.

Pact Server puede ejecutarse en loopback y consultarse mediante un túnel SSH.
El servidor actual usa un token local compartido y no debe exponerse a
empleados o Internet hasta implementar OIDC, membresías, autorización por
proyecto y credenciales independientes de Pact Node.

## Preparar

```sh
cp terraform.tfvars.example terraform.tfvars
terraform init
terraform plan -out=pact.tfplan
terraform apply pact.tfplan
```

`terraform.tfvars`, el estado y los planes están ignorados por Git. No incluyas
claves privadas ni secretos en variables de Terraform.

## Capacidad de Ampere A1

La capacidad Always Free puede agotarse temporalmente. Si OCI responde
`Out of host capacity`, conserva `fault_domain = null` para que el servicio
pueda elegir cualquiera de los tres dominios físicos y vuelve a ejecutar
`terraform plan` y `terraform apply` más tarde. Cambiar a una shape pagada o a
la micro-VM de 1 GB requiere una decisión explícita; esta configuración no lo
hace automáticamente.

## Límites iniciales

Esta es una base económica de una sola VM, no una plataforma con alta
disponibilidad. El volumen de datos se formatea como ext4 y se monta en
`/srv/pact-data`; destruir la instancia no debe implicar destruir ese volumen.
Antes de abrir el servicio se deben añadir:

- dominio y terminación TLS;
- OCI Vault o identidad de instancia para secretos;
- backups de PostgreSQL y del volumen con restauración probada;
- monitorización y alarmas;
- autenticación de equipo y autorización multi-tenant;
- registro seguro de Pact Nodes.
