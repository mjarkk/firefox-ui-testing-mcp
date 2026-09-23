// Builds a compact accessibility tree of the current document.
// Called with the lowest element id the session may hand out, so ids never
// repeat across navigations within one session.
(startId) => {
	const S = window.__bmcp || (window.__bmcp = { next: startId, ids: new WeakMap(), els: new Map() });
	if (S.next < startId) S.next = startId;

	const MAX_NAME = 150;
	const MAX_TEXT = 400;
	const MAX_OPTIONS = 50;

	const SKIP = new Set(['script', 'style', 'noscript', 'template', 'head', 'meta', 'link', 'title', 'base']);
	const INTERACTIVE = new Set([
		'link', 'button', 'textbox', 'searchbox', 'checkbox', 'radio', 'combobox', 'listbox', 'slider',
		'spinbutton', 'switch', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'treeitem', 'option', 'clickable',
	]);
	const NAME_FROM_CONTENT = new Set([
		'button', 'link', 'heading', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'treeitem',
		'cell', 'columnheader', 'rowheader', 'checkbox', 'radio', 'switch', 'tooltip', 'clickable', 'legend', 'caption',
	]);
	const INPUT_ROLES = {
		button: 'button', submit: 'button', reset: 'button', image: 'button', checkbox: 'checkbox', radio: 'radio',
		range: 'slider', number: 'spinbutton', search: 'searchbox', color: 'button', file: 'button',
	};
	const SECTIONING = 'article, aside, main, nav, section';

	const collapse = (s) => (s || '').replace(/\s+/g, ' ').trim();
	const clip = (s, n) => (s.length > n ? s.slice(0, n - 1) + '…' : s);

	function implicitRole(el) {
		const tag = el.localName;
		switch (tag) {
			case 'a': case 'area': return el.hasAttribute('href') ? 'link' : null;
			case 'button': case 'summary': return 'button';
			case 'input': {
				const t = (el.getAttribute('type') || 'text').toLowerCase();
				if (t === 'hidden') return null;
				return INPUT_ROLES[t] || 'textbox';
			}
			case 'select': return el.multiple || el.size > 1 ? 'listbox' : 'combobox';
			case 'textarea': return 'textbox';
			case 'h1': case 'h2': case 'h3': case 'h4': case 'h5': case 'h6': return 'heading';
			case 'img': return el.getAttribute('alt') === '' ? null : 'img';
			case 'nav': return 'navigation';
			case 'main': return 'main';
			case 'header': return el.parentElement && el.parentElement.closest(SECTIONING) ? null : 'banner';
			case 'footer': return el.parentElement && el.parentElement.closest(SECTIONING) ? null : 'contentinfo';
			case 'aside': return 'complementary';
			case 'form': return 'form';
			case 'section': return el.hasAttribute('aria-label') || el.hasAttribute('aria-labelledby') ? 'region' : null;
			case 'article': return 'article';
			case 'ul': case 'ol': case 'menu': return 'list';
			case 'li': return 'listitem';
			case 'table': return 'table';
			case 'tr': return 'row';
			case 'td': return 'cell';
			case 'th': return el.getAttribute('scope') === 'row' ? 'rowheader' : 'columnheader';
			case 'caption': return 'caption';
			case 'dialog': return 'dialog';
			case 'fieldset': case 'details': return 'group';
			case 'legend': return 'legend';
			case 'option': return 'option';
			case 'progress': return 'progressbar';
			case 'meter': return 'meter';
			case 'hr': return 'separator';
			case 'iframe': return 'iframe';
		}
		return null;
	}

	function textById(ids) {
		return ids.split(/\s+/).map((id) => {
			const el = document.getElementById(id);
			return el ? collapse(el.innerText || el.textContent) : '';
		}).filter(Boolean).join(' ');
	}

	function nameOf(el, role) {
		const tag = el.localName;
		const lb = el.getAttribute('aria-labelledby');
		if (lb) {
			const n = textById(lb);
			if (n) return [n, false];
		}
		const al = collapse(el.getAttribute('aria-label'));
		if (al) return [al, false];
		if (tag === 'input' || tag === 'select' || tag === 'textarea') {
			const t = (el.getAttribute('type') || '').toLowerCase();
			if (['button', 'submit', 'reset'].includes(t)) return [collapse(el.value) || (t === 'submit' ? 'Submit' : t === 'reset' ? 'Reset' : ''), false];
			if (t === 'image') return [collapse(el.alt || el.value), false];
			if (el.labels && el.labels.length) {
				const n = Array.from(el.labels).map((l) => collapse(l.innerText || l.textContent)).filter(Boolean).join(' ');
				if (n) return [n, false];
			}
			return [collapse(el.getAttribute('title') || el.getAttribute('placeholder')), false];
		}
		if (tag === 'img' || tag === 'area') return [collapse(el.getAttribute('alt') || el.getAttribute('title')), false];
		if (tag === 'fieldset') {
			const lg = el.querySelector(':scope > legend');
			if (lg) return [collapse(lg.innerText), false];
		}
		if (tag === 'table') {
			const cp = el.querySelector(':scope > caption');
			if (cp) return [collapse(cp.innerText), false];
		}
		if (NAME_FROM_CONTENT.has(role)) {
			const n = collapse(el.innerText || el.textContent);
			if (n) return [n, true];
		}
		return [collapse(el.getAttribute('title')), false];
	}

	function idFor(el) {
		let id = S.ids.get(el);
		if (!id) {
			id = 'e' + S.next++;
			S.ids.set(el, id);
		}
		S.els.set(id, new WeakRef(el));
		return id;
	}

	function childNodesOf(el) {
		if (el.shadowRoot) return el.shadowRoot.childNodes;
		if (el.localName === 'slot') {
			const assigned = el.assignedNodes({ flatten: true });
			if (assigned.length) return assigned;
		}
		return el.childNodes;
	}

	const BR = { br: true };

	function walkChildren(el, parentStyle) {
		const out = [];
		for (const k of childNodesOf(el)) {
			for (const item of walk(k, parentStyle)) out.push(item);
		}
		return out;
	}

	function states(el, role, cs) {
		const st = [];
		const aria = (n) => el.getAttribute('aria-' + n);
		if (el.disabled || aria('disabled') === 'true') st.push('disabled');
		if (role === 'checkbox' || role === 'radio' || role === 'switch' || role === 'menuitemcheckbox' || role === 'menuitemradio') {
			if (el.indeterminate || aria('checked') === 'mixed') st.push('mixed');
			else if (el.checked === true || aria('checked') === 'true') st.push('checked');
		}
		const exp = aria('expanded');
		if (exp === 'true') st.push('expanded');
		else if (exp === 'false') st.push('collapsed');
		if (el.localName === 'details') st.push(el.open ? 'expanded' : 'collapsed');
		if (aria('selected') === 'true' || (el.localName === 'option' && el.selected)) st.push('selected');
		if (aria('pressed') === 'true') st.push('pressed');
		if (aria('current') && aria('current') !== 'false') st.push('current');
		if (el.required || aria('required') === 'true') st.push('required');
		if (el.readOnly || aria('readonly') === 'true') st.push('readonly');
		if (aria('invalid') === 'true') st.push('invalid');
		if (el.localName === 'dialog' && el.open) st.push('open');
		if (document.activeElement === el || (el.shadowRoot && el.shadowRoot.activeElement)) st.push('focused');
		return st;
	}

	function walk(node, parentStyle) {
		if (node.nodeType === Node.TEXT_NODE) {
			if (parentStyle && parentStyle.visibility !== 'visible') return [];
			return node.data ? [{ t: node.data }] : [];
		}
		if (node.nodeType !== Node.ELEMENT_NODE) return [];

		const el = node;
		const tag = el.localName;
		if (SKIP.has(tag)) return [];
		if (el.getAttribute('aria-hidden') === 'true' || el.hasAttribute('inert')) return [];
		if (tag === 'br') return [BR];

		const cs = getComputedStyle(el);
		if (cs.display === 'none') return [];

		if (tag === 'svg') {
			if (cs.visibility !== 'visible') return [];
			const t = el.querySelector(':scope > title');
			const n = collapse(el.getAttribute('aria-label') || (t && t.textContent));
			return n ? [{ r: 'img', n: clip(n, MAX_NAME) }] : [];
		}

		let role = (el.getAttribute('role') || '').trim().split(/\s+/)[0] || implicitRole(el);
		if (role === 'none' || role === 'presentation' || role === 'generic') role = null;
		if (!role && el.isContentEditable && !(el.parentElement && el.parentElement.isContentEditable)) role = 'textbox';

		const visibleSelf = cs.visibility === 'visible';
		let heuristic = false;
		if (!role && visibleSelf) {
			const tabbable = el.tabIndex >= 0 && el.hasAttribute('tabindex');
			const pointer = cs.cursor === 'pointer' && (!parentStyle || parentStyle.cursor !== 'pointer');
			if (tabbable || el.hasAttribute('onclick') || pointer) {
				role = 'clickable';
				heuristic = true;
			}
		}

		const isFormField = tag === 'input' || tag === 'textarea' || tag === 'select';
		const children = isFormField || tag === 'iframe' ? [] : walkChildren(el, cs);
		// A pointer cursor on a wrapper around real controls is not a control itself.
		if (heuristic && children.some(hasId)) role = null;

		if (!role || !visibleSelf) {
			const block = !cs.display.startsWith('inline') && cs.display !== 'contents';
			return block ? [BR, ...children, BR] : children;
		}

		const n = { r: role };
		const [name, fromContent] = nameOf(el, role);
		if (name) n.n = clip(name, MAX_NAME);
		if (INTERACTIVE.has(role) && !(tag === 'option' && el.closest('select'))) n.id = idFor(el);

		if (role === 'heading') {
			const lvl = /^h[1-6]$/.test(tag) ? +tag[1] : +el.getAttribute('aria-level');
			if (lvl) n.lvl = lvl;
		}
		if (role === 'link' && el.hasAttribute('href')) {
			const raw = el.getAttribute('href');
			if (raw && !raw.startsWith('javascript:') && raw !== '#') {
				let href = el.href;
				try {
					const u = new URL(el.href);
					if (u.origin === location.origin) href = u.pathname + u.search + u.hash;
				} catch (_) {}
				n.href = clip(href, 100);
			}
		}
		if (tag === 'iframe') n.href = clip(el.src || '', 100);

		if (tag === 'input' || tag === 'textarea') {
			const t = (el.getAttribute('type') || '').toLowerCase();
			if (!['button', 'submit', 'reset', 'image', 'checkbox', 'radio'].includes(t)) {
				if (el.value) n.v = t === 'password' ? '•'.repeat(Math.min(el.value.length, 12)) : clip(el.value, MAX_TEXT);
				const ph = collapse(el.getAttribute('placeholder'));
				if (ph && ph !== name) n.ph = clip(ph, MAX_NAME);
			}
		} else if (tag === 'select') {
			const opts = Array.from(el.options);
			const sel = opts.filter((o) => o.selected).map((o) => collapse(o.label || o.text));
			if (sel.length) n.v = clip(sel.join(', '), MAX_TEXT);
			n.c = opts.slice(0, MAX_OPTIONS).map((o) => {
				const on = { r: 'option', n: clip(collapse(o.label || o.text), MAX_NAME) };
				if (o.selected) on.st = ['selected'];
				if (o.disabled) on.st = (on.st || []).concat('disabled');
				return on;
			});
			if (opts.length > MAX_OPTIONS) n.c.push({ t: `… ${opts.length - MAX_OPTIONS} more options` });
		} else if (role === 'textbox' && el.isContentEditable) {
			const v = collapse(el.innerText);
			if (v) n.v = clip(v, MAX_TEXT);
		} else if (role === 'slider' || role === 'spinbutton' || role === 'progressbar' || role === 'meter') {
			const v = el.getAttribute('aria-valuetext') || el.getAttribute('aria-valuenow') || el.value;
			if (v !== undefined && v !== null && v !== '') n.v = String(v);
		}

		const st = states(el, role, cs);
		if (st.length) n.st = st.concat(n.st || []);

		const testid = el.getAttribute('data-testid') || el.getAttribute('data-test') || el.getAttribute('data-cy');
		if (testid) n.tid = clip(testid, 60);

		if (!n.c) {
			let c = normalize(children);
			// The name already contains the text content, so only keep what is still actionable.
			if (fromContent || (role === 'textbox' && el.isContentEditable)) {
				c = c.filter(hasId);
				// The kept children describe themselves, repeating their text in the name is noise.
				if (c.length && fromContent) delete n.n;
			}
			if (c.length) n.c = c;
		}
		return [n];
	}

	function hasId(n) {
		return !!n.r && (!!n.id || (n.c || []).some(hasId));
	}

	function normalize(items) {
		const out = [];
		let buf = '';
		const flush = () => {
			const t = collapse(buf);
			if (t) out.push({ t: clip(t, MAX_TEXT) });
			buf = '';
		};
		for (const it of items) {
			if (it.br) flush();
			else if (it.t !== undefined && !it.r) buf += it.t;
			else {
				flush();
				out.push(it);
			}
		}
		flush();
		return out;
	}

	const root = document.body || document.documentElement;
	const tree = root ? normalize(walkChildren(root, getComputedStyle(root))) : [];
	const se = document.scrollingElement || document.documentElement;
	return JSON.stringify({
		tree,
		next: S.next,
		url: location.href,
		title: document.title,
		sx: Math.round(window.scrollX),
		sy: Math.round(window.scrollY),
		sw: se ? se.scrollWidth : 0,
		sh: se ? se.scrollHeight : 0,
	});
}
