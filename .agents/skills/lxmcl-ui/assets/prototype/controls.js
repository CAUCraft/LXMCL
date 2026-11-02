/* Shared controls for the disposable design prototype; no application runtime dependency. */
(() => {
 'use strict';
 const escape = value => String(value).replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
 const button = (label, action, variant = '', graphic = '') => `<button type="button" class="button ${variant}" data-action="${escape(action)}">${graphic}${escape(label)}</button>`;
 // Titles and descriptions are trusted template fragments. User data is escaped by the caller.
 const row = (title, description, control) => `<div class="row"><div class="row-label"><div class="row-title">${title}</div>${description ? `<p>${description}</p>` : ''}</div><div class="row-control">${control}</div></div>`;
 const toggle = (label, checked, attributes = '') => `<input type="checkbox" role="switch" class="switch" aria-label="${escape(label)}" ${checked ? 'checked' : ''} ${attributes}>`;
 const select = (label, options, selected, attributes = '') => `<select class="field" aria-label="${escape(label)}" ${attributes}>${options.map(([value, text]) => `<option value="${escape(value)}" ${String(selected) === String(value) ? 'selected' : ''}>${escape(text)}</option>`).join('')}</select>`;
 const segment = (label, options, selected, attribute = 'segment-value') => `<div class="segment" aria-label="${escape(label)}">${options.map(([value, text]) => `<button type="button" data-${attribute}="${escape(value)}" class="${value === selected ? 'active' : ''}">${escape(text)}</button>`).join('')}</div>`;
 const group = (content, label = '') => `<section class="group" ${label ? `aria-label="${escape(label)}"` : ''}>${content}</section>`;
 const navigation = (label, items, selected, attribute) => `<nav class="context-tabs" aria-label="${escape(label)}"><span class="context-indicator" aria-hidden="true"></span>${items.map(([value,text])=>`<button type="button" data-${attribute}="${escape(value)}" class="${value===selected?'active':''}">${escape(text)}</button>`).join('')}</nav>`;
 const stepper = (label, value, min, max, attribute = '') => `<div class="stepper"><button type="button" data-step="-1" aria-label="减少${escape(label)}">−</button><input type="number" class="field" aria-label="${escape(label)}" min="${min}" max="${max}" value="${value}" ${attribute}><button type="button" data-step="1" aria-label="增加${escape(label)}">+</button></div>`;

 const selector = '.segment,.context-tabs';
 function enhance(root = document) {
  root.querySelectorAll(selector).forEach(group => {
   group.setAttribute('role', 'tablist');
   group.querySelectorAll(':scope > button').forEach(button => {
    const selected = button.classList.contains('active');
    button.setAttribute('role', 'tab');
    button.setAttribute('aria-selected', String(selected));
    button.tabIndex = selected ? 0 : -1;
   });
  });
  root.querySelectorAll('input.switch').forEach(input => input.setAttribute('role', 'switch'));
  root.querySelectorAll('input[type="range"]').forEach(input => {
   const min = Number(input.min || 0), max = Number(input.max || 100);
   input.style.setProperty('--range-fill', `${100 * (Number(input.value) - min) / (max - min)}%`);
  });
  root.querySelectorAll('.context-tabs').forEach(group=>{
   const active=group.querySelector('button.active');
   const indicator=group.querySelector('.context-indicator');
   if(active&&indicator){indicator.style.width=active.offsetWidth+'px';indicator.style.transform=`translateX(${active.offsetLeft}px)`;}
  });
 }
 document.addEventListener('keydown', event => {
  if (!(event.target instanceof Element)) return;
  const group = event.target.closest(selector);
  if (!group || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
  const choices = [...group.querySelectorAll(':scope > button:not(:disabled)')];
  const index = choices.indexOf(event.target);
  if (index < 0) return;
  event.preventDefault();
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? choices.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + choices.length) % choices.length;
  choices[next].focus(); choices[next].click();
 });
 document.addEventListener('click', event => {
  const button = event.target.closest('[data-step]');
  if (!button) return;
  const input = button.parentElement.querySelector('input');
  if (Number(button.dataset.step) > 0) input.stepUp(); else input.stepDown();
  input.dispatchEvent(new Event('input', { bubbles: true }));
  input.dispatchEvent(new Event('change', { bubbles: true }));
 });
 document.addEventListener('input', event => {
  if (event.target.matches('input[type="range"]')) enhance(event.target.parentElement);
 });
 window.LxControls = { button, row, toggle, select, segment, group, stepper, navigation, enhance };
})();
