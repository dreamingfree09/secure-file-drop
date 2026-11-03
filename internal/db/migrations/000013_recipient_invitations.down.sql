-- Explicit rollback removes ONLY new invitation records; existing files remain untouched.
DROP TABLE invitation_send_ledger;
DROP TABLE file_invitations;
