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

function val2str(val) {
	var i,neg
	if (neg=(val<0)) val=-val
	var frac = (val%1e8).toString()
	while (frac.length<8) frac='0'+frac
	for (i=8; i>0 && frac[i-1]=='0'; i--);
	if (i!=8) {
		if (i>0) frac='.'+frac.substr(0,i)
		else frac=''
	} else frac='.'+frac
	var btcs = Math.floor(val/1e8)
	btcs=btcs.toString()+frac
	return neg?('-'+btcs):btcs
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
		return n + " "
	}
	if (n<10e6) {
		return (n/1e3).toFixed(2) + " K"
	}
	if (n<10e9) {
		return (n/1e6).toFixed(2) + " M"
	}
	if (n<10e12) {
		return (n/1e9).toFixed(2) + " G"
	}
	if (n<10e15) {
		return (n/1e12).toFixed(2) + " P"
	}
	if (n<10e18) {
		return (n/1e15).toFixed(2) + " E"
	}
	if (n<10e21) {
		return (n/1e18).toFixed(2) + " Z"
	}
	return (n/1e21).toFixed(2) + " Y"
}

function switch_to_webwallet(name) {
	localStorage.setItem("gocoinWalletSelected", name)
	console.log("re-fetch the wallet")
}

function clear_web_wallet() {
	localStorage.removeItem("gocoinWalletSelected")
	localStorage.removeItem("gocoinWalletId")
}

function switch_to_server_wallet(name) {
	clear_web_wallet()
	document.location='?sid='+sid+'&wal='+name
}

function int2ip(i) {
	var a = (i>>24)&255
	var b = (i>>16)&255
	var c = (i>>8)&255
	var d = i&255
	return a+'.'+b+'.'+c+'.'+d
}

function valid_btc_addr(s) {
	if (s.length<min_btc_addr_len) return false
	for (var i=0; i<s.length; s++) {
		if (b58set.indexOf(s[i])==-1) {
			return false
		}
	}
	return true
}

function fetch_wallet_balance(name) {
	var cont = localStorage.getItem("gocoinWal_"+name).split('\n')
	var wallet = new Array()
	console.log(cont.length)
	for (var i=0; i<cont.length; i++) {
		var ss = cont[i].trim().split(' ')
		if (valid_btc_addr(ss[0])) {
			wallet.push(ss[0])
		}
	}
	console.log(name, wallet)

	var aj = ajax()
	aj.onerror=function() {
		console.log("onerror")
	}

	aj.onload=function() {
		try {
			bal = JSON.parse(aj.responseText)
			console.log(bal)
		} catch (e) {
			console.log(e)
		}
	}
	aj.open("POST", "balance.json", true)
	aj.send(JSON.stringify(wallet))
}

function load_wallet_content() {
	var name = qswal.options[qswal.selectedIndex].text
	//console.log(name)
	fetch_wallet_balance(name)
}
