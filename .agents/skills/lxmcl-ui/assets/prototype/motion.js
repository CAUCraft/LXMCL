/* 日光 04. One outgoing layer and one incoming content surface, interruptible. */
(() => {
 'use strict';
 const running = new WeakMap();
 const snapshots = new Map();
 let timing = value => value;
 const configure = callback => { timing = callback; };
 function stop(element) { running.get(element)?.cancel(); running.delete(element); }
 function animate(element, frames, ms = 160, options = {}) {
  if (!element) return null;
  const previous = running.get(element);
  // Retarget from the currently painted value, not from an entrance pose.
  if (previous && previous.playState !== 'finished' && frames.length > 1) {
   const computed = getComputedStyle(element);
   frames = [{ ...frames[0] }, ...frames.slice(1)];
   Object.keys(frames[0]).forEach(key => {
    if (['opacity', 'transform', 'height'].includes(key)) frames[0][key] = computed[key];
   });
  }
  previous?.cancel();
  if (!timing(ms)) return null;
  const animation = element.animate(frames, { duration: timing(ms), easing: 'cubic-bezier(.22,.8,.24,1)', ...options });
  running.set(element, animation);
  animation.finished.then(() => { if (running.get(element) === animation) running.delete(element); }).catch(() => {});
  return animation;
 }
 function discard(surface) {
  const snapshot = snapshots.get(surface);
  if (!snapshot) return;
  snapshot.animation?.cancel(); snapshot.layer.remove(); snapshots.delete(surface);
 }
 function before(surface, source = surface) {
  if (!surface || !source || source.hidden || !timing(140)) return;
  // Rapid changes share the outgoing layer and the same deadline; incoming content stays live.
  if (snapshots.has(surface)) return;
  if (!source.getClientRects().length) return;
  const bounds = source.getBoundingClientRect();
  const container = surface.getBoundingClientRect();
  const layer = document.createElement('div');
  layer.className = 'transition-layer'; layer.inert = true; layer.setAttribute('aria-hidden', 'true');
  layer.style.cssText = `position:absolute;left:${bounds.left-container.left+surface.scrollLeft}px;top:${bounds.top-container.top+surface.scrollTop}px;width:${bounds.width}px;height:${bounds.height}px;`;
  // Capture the original tree before appending the visual layer to this same surface.
  const originals = [source, ...source.querySelectorAll('*')];
  const copy = source.cloneNode(true);
  // IDs never leak into lookup/labels; snapshots cannot receive focus or intercept clicks.
  [copy, ...copy.querySelectorAll('*')].forEach(node => {
   node.removeAttribute('id'); node.removeAttribute('autofocus');
   node.removeAttribute('aria-controls'); node.removeAttribute('aria-labelledby');
  });
  copy.style.position = 'relative'; copy.style.inset = 'auto';
  copy.style.width = '100%'; copy.style.height = '100%'; copy.style.margin = '0';
  layer.append(copy); surface.append(layer);
  // Restore nested scroll positions so scrolled settings do not jump during the fade.
  const copies = [copy, ...copy.querySelectorAll('*')];
  originals.forEach((node, i) => { copies[i].scrollTop = node.scrollTop; copies[i].scrollLeft = node.scrollLeft; });
  snapshots.set(surface, { layer, animation: null });
 }
 function after(surface, incoming = surface?.firstElementChild) {
  const snapshot = snapshots.get(surface);
  if (!snapshot) return;
  // A renderer may replace the surface's children. Reattach only the visual outgoing layer.
  if (!snapshot.layer.isConnected) surface.append(snapshot.layer);
  if (snapshot.animation) return;
  snapshot.animation = animate(snapshot.layer, [{ opacity: 1 }, { opacity: 0 }], 170);
  if(incoming&&incoming!==snapshot.layer)animate(incoming,[{opacity:.5,transform:'translateY(5px)'},{opacity:1,transform:'none'}],210);
  if (snapshot.animation) snapshot.animation.finished.then(() => discard(surface)).catch(() => {});
  else discard(surface);
 }
 function replace(surface, render, source = surface.firstElementChild) {
  if(!surface)return;
  before(surface, source); render();
  const content=document.createElement('div');content.className='motion-content';
  while(surface.firstChild)content.append(surface.firstChild);
  surface.append(content);after(surface,content);
 }
 function settle() {
  for (const surface of [...snapshots.keys()]) discard(surface);
  document.getAnimations().forEach(animation => { try { animation.finish(); } catch { animation.cancel(); } });
 }
 window.LxMotion = { configure, animate, before, after, replace, settle, stop };
})();
