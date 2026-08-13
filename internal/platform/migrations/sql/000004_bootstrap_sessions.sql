INSERT INTO identity.actors (
    id,
    organization_id,
    kind,
    display_name,
    attributes
) VALUES (
    '00000000-0000-4000-8000-000000000002'::uuid,
    '00000000-0000-4000-8000-000000000001'::uuid,
    'principal',
    'Local administrator',
    '{"bootstrap": true}'::jsonb
) ON CONFLICT (id) DO NOTHING;

INSERT INTO identity.principals (
    id,
    organization_id,
    principal_type,
    external_issuer,
    external_subject
) VALUES (
    '00000000-0000-4000-8000-000000000002'::uuid,
    '00000000-0000-4000-8000-000000000001'::uuid,
    'human',
    'pact.bootstrap',
    'local-administrator'
) ON CONFLICT (id) DO NOTHING;

CREATE UNIQUE INDEX agents_bootstrap_identity_uq
    ON identity.agents (
        organization_id,
        sponsor_principal_id,
        agent_type,
        lower(name)
    );
