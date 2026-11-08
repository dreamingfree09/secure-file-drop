(() => {
  const endpoint = '/uploads/resumable';
  const chunk = 32 * 1024 * 1024;
  const field = id => document.getElementById(id);
  let limit = 0, operation = null, completed = null, pending = null;
  let prepared = null;
  let refreshSequence = 0;

  function status(message) { field('resumableStatus').textContent = message; }

  function active(generation, signal) {
    signal?.throwIfAborted();
    if (!isLoggedIn || generation !== sessionGeneration) throw new DOMException('Session changed', 'AbortError');
  }

  async function request(path, options, generation, signal) {
    active(generation, signal);
    const response = await sessionFetch(path, {...options, signal});
    if (response.status === 401) throw new Error('Your session expired. Sign in again to resume from saved progress.');
    active(generation, signal);
    if (!response.ok) {
      const messages = {
        403: 'Storage quota exceeded.',
        404: 'This transfer is no longer available to your account.',
        409: 'The transfer changed or this chunk differs from the saved file. Reopen the transfer and reselect the original file.',
        410: 'This transfer expired. Start a new transfer.',
        413: 'This file or chunk exceeds the permitted size.',
        422: 'The chunk did not match its checksum.',
        429: 'The server is busy. Resume in a moment.',
        503: 'The server could not confirm this operation. Resume to check committed progress.',
        507: 'The server has insufficient reserved storage capacity.'
      };
      throw new Error(messages[response.status] || 'The upload request failed. Check your connection and try again.');
    }
    const result = await response.json();
    active(generation, signal);
    return result;
  }

  async function digest(blob, generation, signal) {
    const hash = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer());
    active(generation, signal);
    return Array.from(new Uint8Array(hash), byte => byte.toString(16).padStart(2, '0')).join('');
  }

  async function prepare(file, generation, signal) {
    if (prepared?.file === file) return prepared;
    const hashes = [];
    for (let offset = 0; offset < file.size; offset += chunk) {
      status('Checking file chunk ' + (hashes.length + 1) + ' of ' + Math.ceil(file.size / chunk) + '…');
      hashes.push(await digest(file.slice(offset, offset + chunk), generation, signal));
    }
    // SHA-256 of concatenated lowercase hexadecimal chunk digests. This is a
    // browser file-identity manifest. Native SHA-256 still covers the raw bytes.
    const manifest = await digest(new Blob(hashes), generation, signal);
    prepared = {file, hashes, manifest};
    return prepared;
  }

  function renderProgress(upload, speed = 0) {
    field('resumableProgress').value = 100 * upload.offset / upload.size_bytes;
    const committed = (upload.offset / (1024 * 1024)).toFixed(1);
    const total = (upload.size_bytes / (1024 * 1024)).toFixed(1);
    status(committed + ' of ' + total + ' MiB committed' +
      (speed > 0 ? ' · ' + (speed / (1024 * 1024)).toFixed(1) + ' MiB/s' : ''));
  }

  function setBusy(busy) {
    field('resumableStart').disabled = busy;
    field('resumablePause').disabled = !busy;
    field('resumableSaved').disabled = busy;
    field('resumableFile').disabled = busy;
    field('resumableCancel').disabled = busy || !field('resumableSaved').value;
  }

  function clearLink() {
    completed = null;
    field('resumableLink').disabled = true;
    field('resumableLinkValue').value = '';
    field('resumableLinkBox').classList.add('hidden');
  }

  function reset() {
    ++refreshSequence;
    operation?.abort();
    operation = pending = prepared = null;
    field('resumableSaved').replaceChildren(new Option('New transfer', ''));
    field('resumableFile').value = '';
    field('resumableProgress').value = 0;
    clearLink();
    setBusy(false);
    status('');
  }

  async function refresh() {
    const visible = Boolean(limit && isLoggedIn && !isAdmin && globalThis.crypto?.subtle);
    field('resumablePanel').classList.toggle('hidden', !visible);
    if (!visible || operation) return;
    const generation = sessionGeneration, seq = ++refreshSequence;
    const select = field('resumableSaved'), previous = select.value;
    try {
      const saved = await request(endpoint, {}, generation);
      if (seq !== refreshSequence || operation) return;
      select.replaceChildren(new Option('New transfer', ''));
      for (const upload of saved) {
        select.add(new Option(upload.name + ' · ' + upload.state, upload.id));
      }
      if (saved.some(item => item.id === previous)) select.value = previous;
      setBusy(false);
    } catch (error) {
      if (generation === sessionGeneration && seq === refreshSequence && error.name !== 'AbortError') status(error.message);
    }
  }

  function configure(config) {
    if (Number.isSafeInteger(config.resumable_max_bytes) && config.resumable_max_bytes > 0 &&
        config.resumable_max_bytes <= 100 * 1024 ** 3 && config.resumable_chunk_bytes === chunk) {
      limit = config.resumable_max_bytes;
    }
    void refresh();
  }

  async function waitFor(upload, transitional, generation, signal) {
    while (transitional.includes(upload.state)) {
      status(upload.state === 'finalizing' ? 'Verifying the complete object. You can return to this transfer later.' :
        'Waiting for private storage to prepare the transfer…');
      await new Promise(resolve => setTimeout(resolve, 1500));
      upload = await request(endpoint + '/' + upload.id, {}, generation, signal);
    }
    return upload;
  }

  async function transfer() {
    if (!limit || operation || !isLoggedIn || isAdmin) return;
    const file = field('resumableFile').files[0];
    const saved = field('resumableSaved');
    if (!saved.value && (!file || file.size < 1 || file.size > limit)) {
      status('Choose a nonempty file within the configured limit.');
      return;
    }
    const generation = sessionGeneration, controller = new AbortController(), {signal} = controller;
    operation = controller;
    setBusy(true);
    clearLink();
    try {
      let upload;
      if (!saved.value) {
        const identity = await prepare(file, generation, signal);
        if (pending?.file !== file) pending = {file, id: crypto.randomUUID()};
        upload = await request(endpoint, {
          method:'POST', headers:{'Content-Type':'application/json'},
          body:JSON.stringify({request_id:pending.id, name:file.name, content_type:file.type || 'application/octet-stream',
            size_bytes:file.size, manifest_sha256:identity.manifest})
        }, generation, signal);
        saved.add(new Option(upload.name, upload.id));
        saved.value = upload.id;
        pending = null;
      } else {
        upload = await request(endpoint + '/' + saved.value + '?parts=1', {}, generation, signal);
      }
      const committedDigests = upload.part_sha256 || [];
      upload = await waitFor(upload, ['new','finalizing'], generation, signal);
      if (upload.state === 'open') {
        if (!file || file.size !== upload.size_bytes) {
          throw new Error('Reselect the original file to resume.');
        }
        const identity = await prepare(file, generation, signal);
        if (identity.manifest !== upload.manifest_sha256) {
          throw new Error('The selected file differs from the original file. Reselect the original to resume.');
        }
        if (committedDigests.length !== Math.ceil(upload.offset / chunk)) {
          throw new Error('Progress changed in another window. Resume again to verify the current committed chunks.');
        }
        for (let i = 0; i < committedDigests.length; i++) {
          if (identity.hashes[i] !== committedDigests[i]) {
            throw new Error('The selected file differs from the committed chunks. Select the original file.');
          }
        }
        renderProgress(upload);
        while (upload.offset < upload.size_bytes) {
          const offset = upload.offset, part = file.slice(offset, offset + chunk), start = performance.now();
          const sha = identity.hashes[Math.floor(offset / chunk)];
          upload = await request(endpoint + '/' + upload.id, {
            method:'PATCH', headers:{'Content-Type':'application/octet-stream','Upload-Offset':String(offset),'Upload-SHA256':sha}, body:part
          }, generation, signal);
          if (upload.offset !== offset + part.size) throw new Error('Progress changed in another window. Resume again to verify it.');
          renderProgress(upload, part.size / Math.max(.001, (performance.now() - start) / 1000));
        }
        upload = await request(endpoint + '/' + upload.id + '/complete', {method:'POST'}, generation, signal);
        upload = await waitFor(upload, ['finalizing'], generation, signal);
      }
      if (upload.state !== 'complete') {
        throw new Error(upload.state === 'quarantined' ? 'Stored bytes failed verification. The transfer is isolated for review.' :
          'This transfer is no longer writable. Start a new transfer when ready.');
      }
      renderProgress(upload);
      status('Upload verified. Create a link when you are ready to share.');
      completed = upload.id;
      field('resumableLink').disabled = false;
      void loadMyFiles();
    } catch (error) {
      if (generation === sessionGeneration) {
        status(error.name === 'AbortError' ? 'Paused. Resume to check the server’s committed progress.' :
          (error instanceof TypeError ? 'Connection interrupted. Resume to check committed progress.' : error.message));
      } else if (!isLoggedIn && error.message.startsWith('Your session expired')) {
        document.getElementById('loginAlert').textContent = error.message;
      }
    } finally {
      if (operation === controller) {
        operation = null;
        setBusy(false);
        void refresh();
      }
    }
  }

  async function cancel() {
    const id = field('resumableSaved').value;
    if (!id || operation) return;
    const generation = sessionGeneration, controller = new AbortController();
    operation = controller;
    setBusy(true);
    try {
      await request(endpoint + '/' + id, {method:'DELETE'}, generation, controller.signal);
      status('Cancellation accepted. Storage cleanup continues on the server.');
    } catch (error) {
      if (generation === sessionGeneration) status(error.message);
    } finally {
      if (operation === controller) {
        operation = null;
        setBusy(false);
        void refresh();
      }
    }
  }

  async function link() {
    if (!completed) return;
    const generation = sessionGeneration;
    try {
      const result = await request('/links', {method:'POST', headers:{'Content-Type':'application/json'},
        body:JSON.stringify({id:completed,ttl_seconds:86400,share_page:true})}, generation);
      const url = new URL(result.url);
      if (url.protocol !== 'https:' || url.origin !== location.origin || url.pathname !== '/share' ||
          url.username || url.password || url.hash || !url.searchParams.has('token')) throw new Error('The download link could not be verified.');
      field('resumableLinkValue').value = url.href;
      field('resumableLinkBox').classList.remove('hidden');
    } catch (error) {
      if (generation === sessionGeneration) status(error.message);
    }
  }

  field('resumableStart').addEventListener('click', transfer);
  field('resumablePause').addEventListener('click', () => operation?.abort());
  field('resumableCancel').addEventListener('click', cancel);
  field('resumableLink').addEventListener('click', link);
  field('resumableSaved').addEventListener('change', () => {
    field('resumableProgress').value = 0;
    clearLink();
    setBusy(false);
    status('');
  });
  globalThis.SFDResumable = {configure, refresh, reset};
})();
