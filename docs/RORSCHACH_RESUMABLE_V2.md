# Rorschach R2 resumable uploads

The R2 browser and API use authenticated Secure File Drop requests to stage fixed 32 MiB parts in private RustFS. The final part may be shorter. The protocol hard maximum is 107,374,182,400 bytes (100 GiB), or 3,200 parts. The legacy whole-file route and the disabled inherited resumable routes keep their original behavior. The production feature is off by default.

## Enable only in an accepted self-hosted environment

Set both R1_RESUMABLE_MAX_BYTES and R1_RESUMABLE_RESERVED_BYTES in the private Rorschach environment. These map to the corresponding SFD_RESUMABLE variables. Both zero or omitted retain schema 13 and the current 10 MiB whole-file limit. The enabled deployment migrates to schema 14. An existing schema 14 database requires an enabled compatible runtime. The application does not apply a down migration. An upgrade needs an independent paired PostgreSQL and RustFS checkpoint, enough local capacity for incomplete parts and completed bytes, and isolated restore before any production use.

Use the bundled native hash utility and the strict bucket-policy storage mode. The RustFS service remains private. Its scoped identity needs signed multipart create, list, part write, complete and abort, plus object stat and read. Caddy allows exactly 32 MiB bodies only for the new PATCH route. All other requests retain the prior 11 MiB proxy envelope. Keep a reviewed tunnel request cap above 32 MiB and verify request deadlines with representative clients.

## HTTP contract

- POST /uploads/resumable reserves capacity for an owned file. Supply a client-generated UUID request_id, name, content_type and size_bytes. Retrying the same request returns the same session. A conflicting use of the UUID returns 409.
- GET /uploads/resumable lists the caller’s active sessions. GET /uploads/resumable/{id} reports only durable progress. Add parts=1 to retrieve committed SHA-256 digests for bounded prefix verification after reselecting a file.
- PATCH /uploads/resumable/{id} submits a raw application/octet-stream body with exact Content-Length, Upload-Offset and Upload-SHA256. A correct replay returns durable progress without writing a committed part again. A different digest for an already claimed part returns 409.
- POST /uploads/resumable/{id}/complete records intent after all parts are committed. Poll session status. A completed session returns the same file ID and native SHA-256 on repeat requests.
- DELETE /uploads/resumable/{id} requests cancellation before finalization. The worker aborts only known incomplete uploads belonging to the exact owned key. Expired open sessions use the same path. A file already being finalized is preserved for reconciliation.

The existing signed link API accepts the file ID after the verified file row becomes visible. The browser keeps the transfer selection in memory and reads durable progress from the server after sign-in or refresh. When resuming with a reselected file, it checks each already committed chunk against its recorded SHA-256. Its upload and checksum buffers are bounded to one chunk at a time.

## Recovery and operations

An open session reserves quota shared with ordinary uploads. A per-owner creation rate and active-session limit bound reservations, and a deployment-wide limit bounds active staging bytes. These controls complement, but do not replace, disk monitoring and filesystem quotas. An error after a storage call or database commit leaves the durable state as the authority. The server validates the entire bounded request before issuing S3 PUT. It commits the part digest intent before S3 and acknowledges only after committing the ETag and new offset.

Finalization is asynchronous. The worker can adopt the result of an uncertain S3 completion only after verifying the object under the owned key, its exact length and validator, each committed part digest and the bundled native SHA-256. Publishing the file row and releasing the reservation share one transaction. A definitive integrity failure quarantines the transfer and retains its object and reservation for investigation. The protocol never compensates an ambiguous completion by deleting object bytes.

Use separate storage and database backups. Include the schema 14 sessions and incomplete multipart state in any coordinated checkpoint, or drain them first. Off-host recoverability, proxy behavior through the real external route and progressive 10 GiB, 50 GiB and 100 GiB acceptance remain separate rollout gates. The synthetic suite guards storage capacity before its 1 GiB check. A configured maximum is not evidence of a completed transfer.
