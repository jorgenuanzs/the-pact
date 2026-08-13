CREATE UNIQUE INDEX repositories_tenant_remote_active_uq
    ON coordination.repositories (
        organization_id,
        lower(remote_url)
    )
    WHERE remote_url IS NOT NULL
      AND status <> 'archived';
