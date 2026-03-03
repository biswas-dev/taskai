-- Remove all auto-granted project memberships, keep only owners.
-- After this migration, only the project creator (role='owner') retains access.
-- All other members must be re-added explicitly via Project Settings → Members.
DELETE FROM project_members WHERE role != 'owner';
