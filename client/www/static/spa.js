/* ---------------------------------------------------------------------------
   Single Page Application runtime.

   The page head (menu, block counter, theme...) is loaded only once. Each
   page (home, wallet, net...) is then fetched from the node with the
   "X-Gocoin-SPA" request header, which makes the server send only the page's
   content (no html head/tail). The content is put into <main> and its
   <script> tags are executed, the same way as if the page was loaded by the
   browser itself.

   The pages themselves did not need to be rewritten for this. The few
   things they rely on which would not work with dynamically inserted
   content are emulated here:
    * document.addEventListener('DOMContentLoaded', ...) - called after the
      page's scripts have been executed,
    * setTimeout / setInterval / ajax() - remembered, and cancelled when
      leaving the page, so an old page does not keep refreshing itself,
    * blno "lastblock" and dark_light_icon "theme_changed" listeners - dropped
      when leaving the page,
    * <form> submits - sent via fetch(), with the response rendered as a page
      (or saved as a file, for the zip downloads).
   --------------------------------------------------------------------------- */

var spa_gen = 0            // incremented each time a page is unloaded
var spa_intervals = []     // intervals created by the current page
var spa_xhrs = []          // pending XHRs created by the current page
var spa_ready_fns = []     // DOMContentLoaded handlers of the page being loaded
var spa_loaded_scripts = {} // external scripts already present in the document
var spa_main = null        // the <main> element

var _spa_setTimeout = window.setTimeout
var _spa_setInterval = window.setInterval
var _spa_doc_addEventListener = document.addEventListener
var _spa_ajax = ajax

// setTimeout: the callback is silently dropped if the page has been unloaded meanwhile
window.setTimeout = function(fn, ms) {
	var gen = spa_gen, args = Array.prototype.slice.call(arguments, 2)
	return _spa_setTimeout(function() {
		if (gen != spa_gen) return
		if (typeof fn == "function") fn.apply(this, args)
		else (0, eval)(String(fn)) // setTimeout("some_code()", ms)
	}, ms)
}

window.setInterval = function() {
	var id = _spa_setInterval.apply(window, arguments)
	spa_intervals.push(id)
	return id
}

// pages register their init code on DOMContentLoaded - run it after the page's scripts
document.addEventListener = function(type, fn, opts) {
	if (type == "DOMContentLoaded" && document.readyState != "loading") {
		spa_ready_fns.push(fn)
		return
	}
	return _spa_doc_addEventListener.call(document, type, fn, opts)
}

// remember XHRs created by the page (the head uses its own XMLHttpRequest, so it is not affected)
ajax = function() {
	var x = _spa_ajax()
	spa_xhrs = spa_xhrs.filter(function(o) { return o.readyState != 4 })
	spa_xhrs.push(x)
	return x
}

// forms are never submitted the classic way
HTMLFormElement.prototype.submit = function() {
	spa_submit(this, null)
}

_spa_doc_addEventListener.call(document, "submit", function(ev) {
	var f = ev.target
	if (!(f instanceof HTMLFormElement) || ev.defaultPrevented) return
	ev.preventDefault()
	spa_submit(f, ev.submitter)
})

// links to the pages (top menu, help, etc.) are loaded in place
_spa_doc_addEventListener.call(document, "click", function(ev) {
	if (ev.defaultPrevented || ev.button != 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return
	var a = ev.target.closest ? ev.target.closest("a[href]") : null
	if (!a || a.target || a.hasAttribute("download")) return
	if (a.origin != location.origin || !/^\/[a-z]*$/.test(a.pathname)) return
	ev.preventDefault()
	spa_goto(a.pathname + a.search + a.hash)
})

window.addEventListener("popstate", function() {
	spa_goto(location.pathname + location.search + location.hash, false)
})

function spa_get_main() {
	if (!spa_main) spa_main = document.querySelector("main.page")
	return spa_main
}

function spa_page_name(path) {
	var p = path.replace(/^\//, "").replace(/[?#].*$/, "")
	return p == "" ? "home" : p
}

// called by the server, at the beginning of each page
function spa_head_loaded() {
	set_chain_in_sync()
	apply_wallet_on()
}

// called by the server, at the end of each page
function spa_page_time(t) {
	var f = document.querySelector("footer.footer")
	if (f) f.innerText = "Page generated in " + t
}

function spa_unload_page() {
	spa_gen++
	var i
	for (i = 0; i < spa_intervals.length; i++) clearInterval(spa_intervals[i])
	spa_intervals = []
	for (i = 0; i < spa_xhrs.length; i++) {
		var x = spa_xhrs[i]
		x.onload = x.onerror = x.onreadystatechange = null
		try { x.abort() } catch (e) {}
	}
	spa_xhrs = []
	spa_ready_fns = []
	if (typeof closepopup == "function") closepopup()
	// cloning an element drops all the event listeners the page attached to it
	blno.replaceWith(blno.cloneNode(true))
	dark_light_icon.replaceWith(dark_light_icon.cloneNode(true))
	document.body.classList.remove("modal-open")
}

function spa_update_menu(path) {
	var page = spa_page_name(path)
	var items = topmenu.querySelectorAll("a")
	for (var i = 0; i < items.length; i++) {
		items[i].classList.toggle("menuat", items[i].getAttribute("href") == "/" + page)
	}
	helpmenulink.classList.toggle("menuat", page == "help")
	helpmenulink.href = "help#" + page
}

function spa_scroll(hash) {
	if (hash && hash.length > 1) {
		var h = hash.substr(1)
		var el = spa_main.querySelector('[id="' + h + '"], a[name="' + h + '"]')
		if (el) {
			el.scrollIntoView()
			return
		}
	}
	window.scrollTo(0, 0)
}

// executes the <script> tags of the freshly inserted page, in order
function spa_run_scripts(done) {
	var scripts = Array.prototype.slice.call(spa_main.querySelectorAll("script"))
	var gen = spa_gen
	function next() {
		if (gen != spa_gen) return // another page has been loaded meanwhile
		if (scripts.length == 0) return done()
		var old = scripts.shift()
		var s = document.createElement("script")
		for (var i = 0; i < old.attributes.length; i++) {
			s.setAttribute(old.attributes[i].name, old.attributes[i].value)
		}
		var src = old.getAttribute("src")
		if (src) {
			if (spa_loaded_scripts[src]) {
				old.remove()
				next()
				return
			}
			s.onload = function() {
				spa_loaded_scripts[src] = true
				next()
			}
			s.onerror = function() {
				console.log("failed to load", src)
				next()
			}
			old.replaceWith(s) // starts loading
		} else {
			s.textContent = old.textContent
			old.replaceWith(s) // executes it
			next()
		}
	}
	next()
}

function spa_render(html, path, push) {
	spa_get_main()
	spa_unload_page()
	var same = (path == location.pathname + location.search + location.hash)
	if (push === false || same) {
		history.replaceState({spa: true}, "", path)
	} else {
		history.pushState({spa: true}, "", path)
	}
	spa_update_menu(path)
	spa_main.innerHTML = html
	spa_run_scripts(function() {
		var fns = spa_ready_fns
		spa_ready_fns = []
		for (var i = 0; i < fns.length; i++) {
			try { fns[i].call(document, new Event("DOMContentLoaded")) } catch (e) { console.log(e) }
		}
		if (typeof observe_chartboxes == "function") observe_chartboxes()
		spa_scroll(location.hash)
		refreshblock_now() // the page most likely waits for a "lastblock" event
	})
}

function spa_download(resp) {
	var name = resp.url.replace(/[?#].*$/, "").split("/").pop() || "download"
	resp.blob().then(function(blob) {
		var a = document.createElement("a")
		a.href = URL.createObjectURL(blob)
		a.download = name
		document.body.appendChild(a)
		a.click()
		a.remove()
		_spa_setTimeout(function() { URL.revokeObjectURL(a.href) }, 60000)
	})
}

// fetches url (with the given fetch() options) and shows the result as the current page
function spa_fetch(url, init, push) {
	init = init || {}
	init.headers = {"X-Gocoin-SPA": "1"}
	init.credentials = "same-origin"
	spa_get_main().style.opacity = "0.6"
	var hash = url.replace(/^[^#]*/, "") // resp.url does not carry it
	fetch(url, init).then(function(resp) {
		spa_main.style.opacity = ""
		var ct = resp.headers.get("Content-Type") || ""
		if (ct.indexOf("text/html") == 0) {
			var path = location.pathname + location.search + location.hash
			if (init.method != "POST" || resp.redirected) {
				// a page answered directly to a POST (like "shutdown") keeps the current URL
				var u = new URL(resp.url)
				path = u.pathname + u.search + hash
			}
			resp.text().then(function(html) {
				spa_render(html, path, push)
			})
		} else if (ct.indexOf("application/zip") == 0 || ct.indexOf("application/octet-stream") == 0) {
			spa_download(resp)
		} else {
			// e.g. a plain text answer from "cfg" - just refresh the current page
			resp.text().then(function() { spa_reload() })
		}
	}).catch(function(e) {
		spa_main.style.opacity = ""
		console.log(e)
		gc_alert("The node is not responding.", {title: "Error"})
	})
}

// navigates to the page (like document.location = url used to)
function spa_goto(url, push) {
	spa_fetch(url, {method: "GET"}, push)
}

// reloads the current page (like location.reload() used to)
function spa_reload() {
	spa_goto(location.pathname + location.search + location.hash, false)
}

function spa_submit(form, submitter) {
	var fd = new FormData(form)
	if (submitter && submitter.name) fd.append(submitter.name, submitter.value)
	var action = form.getAttribute("action") || (location.pathname + location.search)
	if (form.method.toUpperCase() == "POST") {
		var body = (form.enctype == "multipart/form-data") ? fd : new URLSearchParams(fd)
		spa_fetch(action, {method: "POST", body: body})
	} else {
		spa_goto(action.replace(/\?.*$/, "") + "?" + new URLSearchParams(fd))
	}
}

// the entry page has been loaded by the browser in the classic way - just take a note of it
_spa_doc_addEventListener.call(document, "DOMContentLoaded", function() {
	spa_get_main()
	for (var i = 0; i < document.scripts.length; i++) {
		var src = document.scripts[i].getAttribute("src")
		if (src) spa_loaded_scripts[src] = true
	}
	history.replaceState({spa: true}, "", location.pathname + location.search + location.hash)
})
