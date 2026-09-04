const min_btc_addr_len = 27 // 1111111111111111111114oLvT2
const b58set = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var prvpos = null


function ajax() {
	try { xmlHttp=new XMLHttpRequest(); }
	catch (e) {
		try { xmlHttp=new ActiveXObject("Msxml2.XMLHTTP"); }
		catch (e) {
			try { xmlHttp=new ActiveXObject("Microsoft.XMLHTTP"); }
			catch (e) { alert("AJAX error!"); return null; }
		}
	}
	return xmlHttp;
}

function xval(xml,tag) {
	try {
		return xml.getElementsByTagName(tag)[0].childNodes[0].nodeValue;
	} catch (e) {
		return null
	}
}

function config(q) {
	document.location = 'cfg?sid='+sid+'&'+q
}

function leftpad(v,c,n) {
	v = v.toString()
	while (v.length<n) v=c+v
	return v
}

function rightpad(v,c,n) {
	v = v.toString()
	while (v.length<n) v=v+c
	return v
}

function val2str_pad(val,pad) {
	var i,neg
	if (neg=(val<0)) val=-val
	var frac = (val%1e8).toString()
	while (frac.length<8) frac='0'+frac
	if (pad) {
		frac='.'+frac
	} else {
		for (i=8; i>0 && frac[i-1]=='0'; i--);
		if (i!=8) {
			if (i>0) frac='.'+frac.substr(0,i)
			else frac=''
		} else frac='.'+frac
	}
	var btcs = Math.floor(val/1e8)
	btcs=btcs.toString()+frac
	return neg?('-'+btcs):btcs
}

function val2str(val) {
	return val2str_pad(val,false)
}

function val2int(str) {
	var ss=str.split('.')
	if (ss.length==1) {
		return parseInt(ss[0])*1e8
	} else if (ss.length==2) {
		if (ss[1].length>8) return Number.NaN
		while (ss[1].length<8) ss[1]+='0'
		return parseInt(ss[0])*1e8 + parseInt(ss[1])
	}
	return Number.NaN
}

function tim2str(tim, timeonly) {
	var d = new Date(tim*1000)
	//var timeonly=false
	var res = ''
	if (!timeonly) {
		res += d.getFullYear() + "/" + leftpad(d.getMonth()+1, "0", 2) + "/" + leftpad(d.getDate(), "0", 2) + ' '
	}
	res += leftpad(d.getHours(), "0", 2) + ":" + leftpad(d.getMinutes(), "0", 2) + ":" + leftpad(d.getSeconds(), "0", 2)
	return res
}

function pushtx() {
	gc_prompt("Paste the raw transaction data (hex) to load it into the mempool.", "",
		{title:"Load transaction", multiline:true, ok:"Load", placeholder:"0200000001..."}).then(function(rawtx) {
		if (rawtx==null || rawtx.trim()=="") return
		var form = document.createElement("form")
		form.setAttribute("method", "post")
		form.setAttribute("action", "txs")
		var rtx = document.createElement("input")
		rtx.setAttribute("type", "hidden")
		rtx.setAttribute("name", "rawtx")
		rtx.setAttribute("value", rawtx.trim())
		form.appendChild(rtx)
		document.body.appendChild(form)
		form.submit()
	})
}

function savecfg() {
	document.location='/cfg?savecfg&sid='+sid
}

function bignum(n) {
	if (n<10e3) {
		if ((n%1)===0) {
			return n + ' '
		}
		return n.toFixed(1) + " "
	}
	if (n<10e6) {
		return (n/1e3).toFixed(1) + " K"
	}
	if (n<10e9) {
		return (n/1e6).toFixed(1) + " M"
	}
	if (n<10e12) {
		return (n/1e9).toFixed(1) + " G"
	}
	if (n<10e15) {
		return (n/1e12).toFixed(1) + " T"
	}
	if (n<10e18) {
		return (n/1e15).toFixed(1) + " P"
	}
	if (n<10e21) {
		return (n/1e18).toFixed(1) + " E"
	}
	if (n<10e24) {
		return (n/1e21).toFixed(1) + " Z"
	}
	return (n/1e24).toFixed(2) + " Y"
}

function int2ip(i) {
	var a = (i>>24)&255
	var b = (i>>16)&255
	var c = (i>>8)&255
	var d = i&255
	return a+'.'+b+'.'+c+'.'+d
}

function hex2array(t) {
	if ((t.length & 1)==1) {
		return null
	}
	var pkb = new Uint8Array(t.length/2)
	for (var i = 0; i < t.length; i += 2) {
        var v = parseInt(t.substr(i, 2), 16)
		if (isNaN(v)) return null
		pkb[i/2] = v
	}
	return pkb
}

function valid_pubkey(s) {
	if (s.length == 66 && (s.substr(0,2)=="02" || s.substr(0,2)=="03")) {
		s = s.toLowerCase()
		for (var i=0; i<s.length; i++) {
			var c = s[i]
			if (!(c >= '0' && c <= '9' || c >= 'a' && c <= 'f')) return false
		}
		return true
	}
	return false
}

function valid_bech32_addr(s) {
	return (s.length == 62 || s.length==42) && (s.substr(0,3)=="bc1" || s.substr(0,3)=="tc1")
}

function valid_base58_addr(s) {
	for (var i=0; i<s.length; s++) {
		if (b58set.indexOf(s[i])==-1) {
			return false
		}
	}
	return true
}

function valid_btc_addr(s) {
	try {
		if (s.length<min_btc_addr_len) return false
		if (s[0]=='#') return false
		if (valid_pubkey(s))  return true
		if (valid_bech32_addr(s))  return true
		if (valid_base58_addr(s))  return true
	} catch (e) {
		console.log("valid_btc_addr:", e)
		return false
	}
}

function period2str(upsec) {
	if (upsec<120) {
		return upsec + ' sec'
	}
	var mins = upsec/60
	if (mins<120) {
		return mins.toFixed(1) + ' min'
	}

	var hrs = mins/60
	if (hrs<48) {
		return hrs.toFixed(1) + ' hours'
	}

	var days = hrs/24
	return days.toFixed(1) + ' days'
}

function parse_wallet(s) {
	var wallet = new Array()
	try {
		var cont = s.split('\n')
		for (var i=0; i<cont.length; i++) {
			var line = cont[i].trim()
			var sp_idx = line.indexOf(' ')
			var addr, label
			if (sp_idx==-1) {
				addr = line
				label = ''
			} else {
				addr = line.substr(0, sp_idx)
				label = line.substr(sp_idx+1)
			}
			if (valid_btc_addr(addr)) {
				wallet.push({'addr':addr, 'label':label, 'virgin':cont[i][0]==' '})
			}
		}
	} catch (e) {
		console.log("parse_wallet:", e)
	}
	return wallet
}

function build_wallet_list() {
	var gvi = localStorage.getItem("gocoinWalletId")
	var i

	var names = localStorage.getItem("gocoinWallets").split('|')
	var s = ''
	for (i=0; i<names.length; i++) {
		if (names[i]!="") {
			var content = localStorage.getItem("gocoinWal_"+names[i])
			if (typeof(content)=="string" && content.length > 0) {
				var o = document.createElement("option")
				o.value = o.text = names[i]
				qswal.add(o)
				if (localStorage.getItem("gocoinWalletSelected")==names[i]) {
					qswal.selectedIndex = qswal.length-1
				}
				if (s!='') s+='|'
				s += names[i]
			} else {
				console.log("removing webwallet", names[i])
				localStorage.removeItem("gocoinWal_"+names[i])
			}
		}
	}
	localStorage.setItem("gocoinWallets", s)
}

function quick_switch_wallet() {
	try {
		if (qswal.options.length==0 || qswal.selectedIndex<0 || qswal.options.length<=qswal.selectedIndex) return
		var name = qswal.options[qswal.selectedIndex].text
		localStorage.setItem("gocoinWalletSelected", name)
		var e = document.createEvent("Event")
		e.initEvent("loadwallet", false, false)
		e.name = name
		qswal.dispatchEvent(e)
	} catch (e) {
		console.log("quick_switch_wallet:", e)
	}
}


/* ---------------------------------------------------------------------------
   Popups (#light / #fade) shared by several pages
   --------------------------------------------------------------------------- */
function noscroll() {} // kept for compatibility, no longer needed (popups are position:fixed)

function openpopup() {
	if (typeof light == "undefined" || typeof fade == "undefined") return
	if (!fade["_bound"]) {
		fade["_bound"] = true
		fade.addEventListener('click', closepopup)
		fade.title = 'Click here to close the popup'
	}
	light.style.display = 'block'
	fade.style.display = 'block'
	document.body.classList.add('modal-open')
}

function popup_is_open() {
	return typeof light != "undefined" && light.style.display == 'block'
}

function closepopup_x(fees) {
	if (!popup_is_open()) return
	if (fees && typeof $ != "undefined" && $("#block_fees").length) {
		$("#block_fees").unbind("plothover")
		$("#fees_tooltip").remove()
	}
	light.style.display = 'none'
	fade.style.display = 'none'
	document.body.classList.remove('modal-open')
}

function closepopup() {
	closepopup_x(true)
}

document.addEventListener('keyup', function(event) {
	if (event.key == "Escape" && !document.querySelector('.dlg-overlay')) closepopup()
})

function css(selector, property, value) {
	for (var i=0; i<document.styleSheets.length;i++) {//Loop through all styles
		//Try add rule
		try {
			document.styleSheets[i].insertRule(selector+ ' {'+property+':'+value+'}', document.styleSheets[i].cssRules.length);
		} catch(err) {try { document.styleSheets[i].addRule(selector, property+':'+value);} catch(err) {}}//IE
	}
}

/* ---------------------------------------------------------------------------
   Clipboard + toast
   --------------------------------------------------------------------------- */
var _toast_timer = null
function gc_toast(msg) {
	var t = document.getElementById('gc_toast')
	if (!t) {
		t = document.createElement('div')
		t.id = 'gc_toast'
		t.className = 'toast'
		document.body.appendChild(t)
	}
	t.textContent = msg
	void t.offsetWidth
	t.classList.add('show')
	if (_toast_timer) clearTimeout(_toast_timer)
	_toast_timer = setTimeout(function() { t.classList.remove('show') }, 1400)
}

function copy_text(txt) {
	function fallback() {
		try {
			autocopy.style.display = 'inline'
			autocopy.value = txt
			autocopy.select()
			document.execCommand('copy')
			autocopy.style.display = 'none'
		} catch (e) {}
	}
	if (navigator.clipboard && window.isSecureContext) {
		navigator.clipboard.writeText(txt).catch(fallback)
	} else {
		fallback()
	}
	gc_toast("Copied to clipboard")
}

function copyonclick(e) {
	var el = e.currentTarget || e.srcElement
	var txt = el["text2copy"]
	if (typeof txt == "undefined" && e.srcElement) txt = e.srcElement["text2copy"]
	if (typeof txt == "undefined") return
	copy_text(txt)
	e.stopPropagation()
}

/* ---------------------------------------------------------------------------
   Dialogs (replacement for prompt / confirm / alert)
   gc_prompt(msg, def, opts) -> Promise<string|null>
   gc_confirm(msg, opts)     -> Promise<bool>
   gc_alert(msg, opts)       -> Promise<void>
   opts: {title, ok, cancel, danger, multiline, placeholder, mono}
   --------------------------------------------------------------------------- */
function gc_dialog(kind, msg, def, opts) {
	opts = opts || {}
	return new Promise(function(resolve) {
		var ov = document.createElement('div')
		ov.className = 'dlg-overlay'
		var box = document.createElement('div')
		box.className = 'dlg' + (opts.danger ? ' danger' : '') + (opts.multiline ? ' wide' : '')
		box.setAttribute('role', kind=='alert' ? 'alertdialog' : 'dialog')
		box.setAttribute('aria-modal', 'true')

		var title = opts.title || (kind=='confirm' ? 'Please confirm' : (kind=='alert' ? 'Notice' : 'Input needed'))
		var h = document.createElement('div')
		h.className = 'dlg-title'
		h.textContent = title
		box.appendChild(h)

		var m = document.createElement('div')
		m.className = 'dlg-msg'
		if (opts.html) m.innerHTML = msg; else m.textContent = msg
		box.appendChild(m)

		var input = null
		if (kind=='prompt') {
			input = document.createElement(opts.multiline ? 'textarea' : 'input')
			if (!opts.multiline) input.type = 'text'
			input.value = (def==null) ? '' : def
			if (opts.placeholder) input.placeholder = opts.placeholder
			input.setAttribute('autocomplete', 'off')
			input.setAttribute('spellcheck', 'false')
			box.appendChild(input)
		}

		var act = document.createElement('div')
		act.className = 'dlg-actions'
		var okb = document.createElement('button')
		okb.type = 'button'
		okb.className = 'btn ' + (opts.danger ? 'danger' : 'primary')
		okb.textContent = opts.ok || 'OK'
		var cancel = null
		if (kind!='alert') {
			cancel = document.createElement('button')
			cancel.type = 'button'
			cancel.className = 'btn'
			cancel.textContent = opts.cancel || 'Cancel'
			act.appendChild(cancel)
		}
		act.appendChild(okb)
		box.appendChild(act)
		ov.appendChild(box)
		document.body.appendChild(ov)

		var prev_focus = document.activeElement
		var done = false
		function finish(val) {
			if (done) return
			done = true
			ov.classList.add('closing')
			document.removeEventListener('keydown', onkey, true)
			setTimeout(function() {
				if (ov.parentNode) ov.parentNode.removeChild(ov)
				try { if (prev_focus && prev_focus.focus) prev_focus.focus() } catch (e) {}
				resolve(val)
			}, 140)
		}
		function ok() {
			if (kind=='prompt') finish(input.value)
			else if (kind=='confirm') finish(true)
			else finish(undefined)
		}
		function no() {
			if (kind=='prompt') finish(null)
			else if (kind=='confirm') finish(false)
			else finish(undefined)
		}
		function onkey(ev) {
			if (ev.key=='Escape') { ev.preventDefault(); ev.stopPropagation(); no() }
			else if (ev.key=='Enter' && !(opts.multiline && !ev.ctrlKey && !ev.metaKey)) {
				if (kind!='prompt' || document.activeElement==input || document.activeElement==okb || document.activeElement==box) {
					ev.preventDefault(); ev.stopPropagation(); ok()
				}
			} else if (ev.key=='Tab') {
				// keep focus inside the dialog
				var f = box.querySelectorAll('input,textarea,button')
				var first = f[0], last = f[f.length-1]
				if (ev.shiftKey && document.activeElement==first) { ev.preventDefault(); last.focus() }
				else if (!ev.shiftKey && document.activeElement==last) { ev.preventDefault(); first.focus() }
			}
		}
		document.addEventListener('keydown', onkey, true)
		okb.addEventListener('click', ok)
		if (cancel) cancel.addEventListener('click', no)
		ov.addEventListener('mousedown', function(ev) { if (ev.target==ov) no() })

		setTimeout(function() {
			if (input) { input.focus(); if (!opts.multiline) input.select() }
			else okb.focus()
		}, 20)
	})
}
function gc_prompt(msg, def, opts) { return gc_dialog('prompt', msg, def, opts) }
function gc_confirm(msg, opts) { return gc_dialog('confirm', msg, null, opts) }
function gc_alert(msg, opts) { return gc_dialog('alert', msg, null, opts) }

// use as: onsubmit="return gc_confirm_submit(this, event, 'Really?', {ok:'Yes'})"
function gc_confirm_submit(form, ev, msg, opts) {
	if (form["_gc_confirmed"]) {
		form["_gc_confirmed"] = false
		return true
	}
	ev.preventDefault()
	var submitter = ev.submitter
	gc_confirm(msg, opts).then(function(yes) {
		if (!yes) return
		form["_gc_confirmed"] = true
		if (submitter && form.requestSubmit) form.requestSubmit(submitter)
		else form.submit()
	})
	return false
}

/* ---------------------------------------------------------------------------
   Theme helpers for charts
   --------------------------------------------------------------------------- */
function theme_var(name) {
	return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function flot_points_fill_color() {
	return theme_var('--surface') || (dark_mode ? 'black' : 'white')
}

// merge theme-aware grid colors into flot options
function flot_theme(opts) {
	if (!opts.grid) opts.grid = {}
	opts.grid.color = theme_var('--text-3')
	opts.grid.tickColor = theme_var('--line')
	opts.grid.borderColor = theme_var('--line')
	opts.grid.borderWidth = 1
	opts.grid.backgroundColor = 'transparent'
	return opts
}

// themed tooltips for flot charts
function flot_tooltip(id, x, y, contents, cls) {
	var t = document.createElement('div')
	t.id = id
	t.className = 'flot-tip' + (cls ? ' '+cls : '')
	t.innerHTML = contents
	t.style.top = (y - 30) + 'px'
	t.style.left = (x + 8) + 'px'
	document.body.appendChild(t)
	// keep it inside the viewport
	var r = t.getBoundingClientRect()
	if (r.right > window.innerWidth - 8) t.style.left = (x - r.width - 8) + 'px'
	if (r.top < 4) t.style.top = (y + 12) + 'px'
}
