const min_btc_addr_len = 27 // 1111111111111111111114oLvT2
const b58set = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

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
		return NaN
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
	var rawtx = prompt("Enter raw transaction data (hexdump)")
	if (rawtx!=null) {
		var form = document.createElement("form")
		form.setAttribute("method", "post")
		form.setAttribute("action", "txs")
		var rtx = document.createElement("input")
		rtx.setAttribute("type", "hidden")
		rtx.setAttribute("name", "rawtx")
		rtx.setAttribute("value", rawtx)
		form.appendChild(rtx)
		document.body.appendChild(form)
		form.submit()
	}
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

function valid_btc_addr(s) {
	try {
		if (s.length<min_btc_addr_len) return false
		for (var i=0; i<s.length; s++) {
			if (b58set.indexOf(s[i])==-1) {
				return false
			}
		}
		return true
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

function noscroll() {
	scroll(0,0)
}

function closepopup() {
	light.style.display='none'
	fade.style.display='none'
	window.scrollTo(0,prvpos)
	document.removeEventListener("scroll", noscroll)
}
