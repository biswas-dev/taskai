// go-wiki annotations module
// Exposes window.GoWikiAnnotations with:
//   attach(previewEl, callbacks) -> detachFn
//   apply(previewEl, annotations, selectedId)
//
// callbacks: {
//   onAnnotationCreate({ startOffset, endOffset, selectedText, color }),
//   onAnnotationClick(annotationId)
// }
(function () {
  'use strict';

  var COLORS = {
    yellow: 'rgba(255,236,61,0.35)',
    blue:   'rgba(96,165,250,0.35)',
    green:  'rgba(74,222,128,0.35)',
    red:    'rgba(248,113,113,0.35)',
  };

  var COLORS_SELECTED = {
    yellow: 'rgba(255,236,61,0.65)',
    blue:   'rgba(96,165,250,0.65)',
    green:  'rgba(74,222,128,0.65)',
    red:    'rgba(248,113,113,0.65)',
  };

  // --------------------------------------------------------------------------
  // getTextOffset — character offset of targetNode:targetOffset within root
  // --------------------------------------------------------------------------
  function getTextOffset(root, targetNode, targetOffset) {
    var count = 0;
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      var node = walker.currentNode;
      if (node === targetNode) return count + targetOffset;
      count += (node.textContent || '').length;
    }
    return count;
  }

  // --------------------------------------------------------------------------
  // applyHighlights — injects <mark data-ann-id> for each annotation
  // --------------------------------------------------------------------------
  function applyHighlights(container, annotations, selectedId) {
    // Remove previous marks
    container.querySelectorAll('mark[data-ann-id]').forEach(function (mark) {
      var parent = mark.parentNode;
      if (!parent) return;
      while (mark.firstChild) parent.insertBefore(mark.firstChild, mark);
      parent.removeChild(mark);
    });

    if (!annotations || !annotations.length) return;

    // Apply in reverse start_offset order so earlier offsets stay valid
    var sorted = annotations.slice().sort(function (a, b) {
      return b.start_offset - a.start_offset;
    });

    sorted.forEach(function (ann) {
      if (ann.resolved) return;
      try {
        var charCount = 0;
        var startNode = null, startOff = 0, endNode = null, endOff = 0;
        var walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
        while (walker.nextNode()) {
          var node = walker.currentNode;
          var len = (node.textContent || '').length;
          if (!startNode && charCount + len > ann.start_offset) {
            startNode = node;
            startOff = ann.start_offset - charCount;
          }
          if (!endNode && charCount + len >= ann.end_offset) {
            endNode = node;
            endOff = ann.end_offset - charCount;
            break;
          }
          charCount += len;
        }
        if (!startNode || !endNode) return;

        var range = document.createRange();
        range.setStart(startNode, startOff);
        range.setEnd(endNode, endOff);

        var mark = document.createElement('mark');
        mark.dataset.annId = String(ann.id);
        mark.style.cssText = [
          'background:' + (selectedId === ann.id ? COLORS_SELECTED[ann.color] : COLORS[ann.color]),
          'border-radius:2px',
          'cursor:pointer',
          'transition:background 0.15s',
        ].join(';');

        var fragment = range.extractContents();
        mark.appendChild(fragment);
        range.insertNode(mark);
      } catch (e) {
        // Range invalid — annotation may pre-date content changes
      }
    });
  }

  // --------------------------------------------------------------------------
  // Color picker popup
  // --------------------------------------------------------------------------
  var _popupEl = null;

  function removePopup() {
    if (_popupEl && _popupEl.parentNode) _popupEl.parentNode.removeChild(_popupEl);
    _popupEl = null;
  }

  function showPopup(x, y, startOffset, endOffset, selectedText, onPick) {
    removePopup();

    var el = document.createElement('div');
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-label', 'Choose highlight color');
    el.style.cssText = [
      'position:fixed',
      'z-index:9999',
      'display:flex',
      'align-items:center',
      'gap:6px',
      'background:#1e1e2e',
      'border:1px solid rgba(255,255,255,0.12)',
      'border-radius:8px',
      'padding:6px 10px',
      'box-shadow:0 4px 24px rgba(0,0,0,0.4)',
      'left:' + Math.max(8, x) + 'px',
      'top:' + Math.max(8, y) + 'px',
    ].join(';');

    var label = document.createElement('span');
    label.textContent = 'Highlight:';
    label.style.cssText = 'font-size:11px;color:rgba(255,255,255,0.5);font-family:sans-serif;white-space:nowrap';
    el.appendChild(label);

    ['yellow', 'blue', 'green', 'red'].forEach(function (color) {
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.title = color.charAt(0).toUpperCase() + color.slice(1);
      btn.setAttribute('aria-label', 'Highlight ' + color);
      btn.style.cssText = [
        'width:18px',
        'height:18px',
        'border-radius:50%',
        'cursor:pointer',
        'border:2px solid ' + COLORS_SELECTED[color],
        'background:' + COLORS_SELECTED[color],
        'transition:transform 0.1s',
        'outline:none',
        'padding:0',
        'flex-shrink:0',
      ].join(';');
      btn.onmouseenter = function () { btn.style.transform = 'scale(1.25)'; };
      btn.onmouseleave = function () { btn.style.transform = 'scale(1)'; };
      btn.onclick = function (e) {
        e.stopPropagation();
        removePopup();
        window.getSelection && window.getSelection().removeAllRanges();
        onPick({ startOffset: startOffset, endOffset: endOffset, selectedText: selectedText, color: color });
      };
      el.appendChild(btn);
    });

    var cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.title = 'Cancel';
    cancelBtn.setAttribute('aria-label', 'Cancel highlight');
    cancelBtn.style.cssText = [
      'background:none',
      'border:none',
      'cursor:pointer',
      'color:rgba(255,255,255,0.4)',
      'font-size:14px',
      'line-height:1',
      'padding:0 0 0 2px',
      'font-family:sans-serif',
    ].join(';');
    cancelBtn.textContent = '\u00d7'; // ×
    cancelBtn.onmouseenter = function () { cancelBtn.style.color = 'rgba(255,255,255,0.9)'; };
    cancelBtn.onmouseleave = function () { cancelBtn.style.color = 'rgba(255,255,255,0.4)'; };
    cancelBtn.onclick = function (e) { e.stopPropagation(); removePopup(); };
    el.appendChild(cancelBtn);

    document.body.appendChild(el);
    _popupEl = el;
  }

  // --------------------------------------------------------------------------
  // attach — binds mouseup to previewEl, returns a detach function
  // --------------------------------------------------------------------------
  function attach(previewEl, callbacks) {
    function handleMouseUp(e) {
      // Click on existing mark → fire onAnnotationClick
      var target = e.target;
      var mark = target && target.closest ? target.closest('mark[data-ann-id]') : null;
      if (mark) {
        var annId = parseInt(mark.dataset.annId, 10);
        if (!isNaN(annId) && callbacks.onAnnotationClick) {
          callbacks.onAnnotationClick(annId);
          return;
        }
      }

      if (!callbacks.onAnnotationCreate) return;

      var sel = window.getSelection();
      if (!sel || sel.isCollapsed || !sel.rangeCount) {
        removePopup();
        return;
      }

      var range = sel.getRangeAt(0);
      if (!previewEl.contains(range.commonAncestorContainer)) {
        removePopup();
        return;
      }

      var selectedText = sel.toString().trim();
      if (!selectedText) return;

      var startOffset = getTextOffset(previewEl, range.startContainer, range.startOffset);
      var endOffset   = getTextOffset(previewEl, range.endContainer,   range.endOffset);
      if (startOffset >= endOffset) return;

      var rect = range.getBoundingClientRect();
      var x = rect.left + rect.width / 2 - 120;
      var y = rect.top - 52;

      showPopup(x, y, startOffset, endOffset, selectedText, function (data) {
        callbacks.onAnnotationCreate(data);
      });
    }

    function handleDocMouseDown(e) {
      if (_popupEl && !_popupEl.contains(e.target)) removePopup();
    }

    previewEl.addEventListener('mouseup', handleMouseUp);
    document.addEventListener('mousedown', handleDocMouseDown);

    return function detach() {
      previewEl.removeEventListener('mouseup', handleMouseUp);
      document.removeEventListener('mousedown', handleDocMouseDown);
      removePopup();
    };
  }

  // --------------------------------------------------------------------------
  // Public API
  // --------------------------------------------------------------------------
  window.GoWikiAnnotations = {
    attach: attach,
    apply:  applyHighlights,
  };
})();
