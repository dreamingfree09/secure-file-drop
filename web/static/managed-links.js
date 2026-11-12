/* Account-bound metadata only. Existing bearer links are never reconstructed. */
(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
  let enabled = false, epoch = 0, request = 0, next = '', pending = null, timer;
  let busy = false;
  const current = (e, g) => e === epoch && g === sessionGeneration && enabled && isLoggedIn && !isAdmin;
  function message(text, error = false) {
    $('managedStatus').textContent = text;
    $('managedStatus').dataset.error = String(error);
  }
  function controls(disabled) {
    busy = disabled;
    for (const id of ['managedRefresh', 'managedFile', 'managedCreate', 'managedMore']) $(id).disabled = disabled;
  }
  function clearIssued() {
    $('managedIssued').hidden = true;
    $('managedURL').value = '';
  }
  function reset() {
    epoch++; request++; pending = null; next = '';
    clearTimeout(timer);
    if ($('managedConfirm').open) $('managedConfirm').close();
    $('managedConfirmName').textContent = '';
    $('managedConfirmYes').disabled = false;
    $('managedList').replaceChildren();
    $('managedFile').replaceChildren(new Option('All files', ''));
    $('managedMore').hidden = true;
    $('managedLinksPanel').hidden = true;
    clearIssued(); controls(false); message('');
  }
  function configure(config) {
    enabled = config.managed_links_enabled === true;
    if (!enabled) reset();
    else if (isLoggedIn && !isAdmin) void refresh();
  }
  function files(items) {
    if (!enabled || !isLoggedIn || isAdmin) return;
    const selected = $('managedFile').value;
    const choices = [new Option('All files', '')];
    for (const file of items) {
      if (uuid.test(file.id) && ['hashed', 'ready'].includes(file.status)) choices.push(new Option(file.orig_name, file.id));
    }
    $('managedFile').replaceChildren(...choices);
    $('managedFile').value = choices.some(option => option.value === selected) ? selected : '';
    if (!busy) void refresh();
  }
  function validRow(row) {
    return row && uuid.test(row.id) && uuid.test(row.file_id) && typeof row.file_name === 'string' &&
      ['active', 'expired', 'revoked'].includes(row.state) &&
      Number.isFinite(Date.parse(row.created_at)) && Number.isFinite(Date.parse(row.expires_at)) &&
      (!row.revoked_at || Number.isFinite(Date.parse(row.revoked_at)));
  }
  function append(row) {
    const card = document.createElement('article');
    card.className = 'managed-card'; card.dataset.linkId = row.id;
    const heading = document.createElement('h3'); heading.textContent = row.file_name;
    const badge = document.createElement('span');
    badge.className = 'managed-state managed-state-' + row.state;
    badge.textContent = row.state[0].toUpperCase() + row.state.slice(1);
    const details = document.createElement('dl');
    for (const [label, value] of [['Link ID', row.id], ['Created', new Date(row.created_at).toLocaleString()],
      ['Expires', new Date(row.expires_at).toLocaleString()], ...(row.revoked_at ? [['Revoked', new Date(row.revoked_at).toLocaleString()]] : [])]) {
      const term = document.createElement('dt'), description = document.createElement('dd');
      term.textContent = label; description.textContent = value; details.append(term, description);
    }
    card.append(heading, badge, details);
    if (row.state === 'active') {
      const revoke = document.createElement('button');
      revoke.type = 'button'; revoke.className = 'btn btn-danger'; revoke.textContent = 'Revoke link';
      revoke.addEventListener('click', () => {
        if (busy || !enabled || !isLoggedIn) return;
        pending = {id: row.id, epoch, generation: sessionGeneration};
        $('managedConfirmName').textContent = 'Revoke this link for “' + row.file_name + '”?';
        $('managedConfirm').showModal(); $('managedConfirmCancel').focus();
      });
      card.append(revoke);
    }
    $('managedList').append(card);
  }
  async function refresh(more = false) {
    if (!enabled || !isLoggedIn || isAdmin) return;
    const e = epoch, g = sessionGeneration, sequence = ++request;
    clearTimeout(timer); controls(true);
    $('managedLinksPanel').hidden = false;
    // Fail closed visually too: do not present stale 'active' badges after a failed refresh.
    if (!more) { $('managedList').replaceChildren(); next = ''; $('managedMore').hidden = true; }
    message('Checking your links…');
    const query = new URLSearchParams();
    if ($('managedFile').value) query.set('file_id', $('managedFile').value);
    if (more && next) query.set('before', next);
    try {
      const response = await sessionFetch('/managed-links?' + query);
      if (!current(e, g) || sequence !== request) return;
      if (!response.ok) throw new Error('listing');
      const rows = await response.json();
      if (!current(e, g) || sequence !== request) return;
      if (!Array.isArray(rows) || rows.length > 50 || !rows.every(validRow)) throw new Error('listing');
      const cursor = response.headers.get('X-Next-Cursor') || '';
      if (cursor && !uuid.test(cursor)) throw new Error('listing');
      for (const row of rows) append(row);
      next = cursor; $('managedMore').hidden = !next;
      message($('managedList').children.length ? 'Showing server-verified link status. Times use your local timezone.' : 'No sharing links found for these files.');
    } catch {
      if (current(e, g) && sequence === request) {
        $('managedList').replaceChildren(); next = ''; $('managedMore').hidden = true;
        message('Could not verify your links. Refresh to try again.', true);
      }
    } finally {
      if (current(e, g) && sequence === request) {
        controls(false);
        // Refresh the current page from server time, including expiry. Older pages
        // can be requested again; never infer authorisation from the local clock.
        timer = setTimeout(() => void refresh(), 60000);
      }
    }
  }
  async function revoke() {
    const target = pending;
    if (!target || !current(target.epoch, target.generation) || busy) return;
    clearTimeout(timer); controls(true); $('managedConfirmYes').disabled = true;
    try {
      const response = await sessionFetch('/managed-links/' + target.id, {method: 'DELETE'});
      if (!current(target.epoch, target.generation)) return;
      $('managedConfirm').close(); pending = null;
      clearIssued();
      if (response.status !== 204) throw new Error('revocation');
      await refresh();
    } catch {
      if (current(target.epoch, target.generation)) {
        if ($('managedConfirm').open) $('managedConfirm').close();
        pending = null; next = ''; $('managedMore').hidden = true; $('managedList').replaceChildren();
        message('Revocation could not be confirmed. Refresh to check the server before sharing again.', true);
      }
    } finally {
      if (current(target.epoch, target.generation)) { controls(false); $('managedConfirmYes').disabled = false; }
    }
  }
  async function create() {
    if (busy || !enabled || !isLoggedIn || isAdmin) return;
    const file = $('managedFile').value;
    if (!uuid.test(file)) { message('Choose an uploaded file to create a link.', true); return; }
    const e = epoch, g = sessionGeneration;
    clearTimeout(timer); clearIssued(); controls(true);
    try {
      const response = await sessionFetch('/links', {method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({id: file, ttl_seconds: 3600, share_page: true})});
      if (!current(e, g)) return;
      if (!response.ok) throw new Error('issuance');
      const value = await response.json();
      if (!current(e, g)) return;
      const url = new URL(value.url);
      if (!uuid.test(value.link_id) || url.origin !== location.origin || url.pathname !== '/share' ||
          url.username || url.password || url.hash || url.searchParams.getAll('token').length !== 1 ||
          [...url.searchParams.keys()].some(key => key !== 'token') || !url.searchParams.get('token')) throw new Error('issuance');
      await refresh();
      if (!current(e, g)) return;
      $('managedURL').value = url.href; $('managedIssued').hidden = false;
      message('Link created for one hour. Copy it now; it cannot be recovered from the list.');
    } catch {
      if (current(e, g)) message('Could not confirm link creation. Refresh before trying again; no link is shown.', true);
    } finally { if (current(e, g)) controls(false); }
  }
  $('managedRefresh').addEventListener('click', () => void refresh());
  $('managedMore').addEventListener('click', () => void refresh(true));
  $('managedFile').addEventListener('change', () => { clearIssued(); void refresh(); });
  $('managedCreate').addEventListener('click', () => void create());
  $('managedConfirmYes').addEventListener('click', () => void revoke());
  $('managedConfirmCancel').addEventListener('click', () => { pending = null; $('managedConfirm').close(); });
  $('managedConfirm').addEventListener('cancel', () => { pending = null; });
  $('managedCopy').addEventListener('click', async () => {
    const e = epoch, g = sessionGeneration, value = $('managedURL').value;
    if (!current(e, g) || !value) return;
    try { await navigator.clipboard.writeText(value); if (current(e, g)) message('Link copied. Anyone with it can download until expiry or revocation.'); }
    catch { if (current(e, g)) { $('managedURL').select(); message('Clipboard unavailable. Copy the selected link manually.', true); } }
  });
  globalThis.SFDManagedLinks = {configure, files, refresh, reset};
})();
