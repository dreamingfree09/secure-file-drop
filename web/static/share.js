(() => {
  'use strict';
  const status = document.getElementById('share-status');
  const form = document.getElementById('download-form');
  const tokenField = document.getElementById('share-token');
  const tokens = new URL(window.location.href).searchParams.getAll('token');
  const valid = tokens.length === 1 && tokens[0].length <= 4096 &&
    /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(tokens[0]);
  if (!valid) {
    status.textContent = 'This sharing link is missing or invalid. Ask the sender for a new link.';
    return;
  }
  tokenField.value = tokens[0];
  form.hidden = false;
  status.textContent = 'Ready to download. The file will be checked when you select the button.';
})();
