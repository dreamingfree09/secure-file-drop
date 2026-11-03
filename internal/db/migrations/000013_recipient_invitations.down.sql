-- Explicit rollback removes ONLY new invitation records; existing files remain untouched.
DROP TABLE file_invitations;
