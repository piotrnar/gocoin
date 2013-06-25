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

function raw_load(id, tit) {
	var aj = ajax()
	aj.onreadystatechange=function() {
		if(xmlHttp.readyState==4) {
			rawtit.innerHTML = tit
			rawdiv.innerHTML = xmlHttp.responseText
		}
	}
	xmlHttp.open("GET","raw_"+id, true);
	xmlHttp.send(null);
}

function xval(xml,tag) {
	return xml.getElementsByTagName(tag)[0].childNodes[0].nodeValue;
}

function add_ths(tab, hdrs) {
	var row = tab.insertRow()
	for (var i=0; i<hdrs.length; i++) {
		var th = document.createElement('th')
		th.innerHTML = hdrs[i]
		row.appendChild(th)
	}
}

function get_maturity(t) {
	return ((((new Date()).getTime()/1000) - parseInt(t))/60).toFixed(0) + '&nbsp;min'
}

function val2reason(r) {
	switch (parseInt(r)) {
		case 101: return "TOO_BIG"
		case 102: return "FORMAT"
		case 103: return "LEN_MISMATCH"
		case 104: return "NO_INPUTS"
		case 201: return "DOUBLE_SPEND"
		case 202: return "NO_INPUT"
		case 203: return "DUST"
		case 204: return "OVERSPEND"
		case 205: return "LOW_FEE"
		case 206: return "SCRIPT_FAIL"
	}
	return r
}


function show_txs2s() {
	var aj = ajax()
	aj.onreadystatechange=function() {
		if(xmlHttp.readyState==4) {
			while (txs2s.rows.length>1)  txs2s.deleteRow(1)
			txs = aj.responseXML.getElementsByTagName('tx')
			for (var i=0; i<txs.length; i++) {
				var c,row = txs2s.insertRow(-1)
				row.className='hov'

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = (i+1).toString()

				c = row.insertCell(-1)
				c.className ='mono'
				c.innerHTML = xval(txs[i], 'id')

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = get_maturity(xval(txs[i], 'time'))

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = xval(txs[i], 'len')

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = xval(txs[i], 'sentcnt')

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = (parseFloat(xval(txs[i], 'volume'))/1e8).toFixed(8)

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = (parseFloat(xval(txs[i], 'fee'))/1e8).toFixed(8)

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = (parseFloat(xval(txs[i], 'fee'))/parseFloat(xval(txs[i], 'len'))).toFixed(1)
			}
			txs2s.style.display = 'table'
		}
	}
	txs2s.style.display = txsre.style.display = 'none'
	xmlHttp.open("GET","txs2s.xml", true);
	xmlHttp.send(null);

}

function show_txsre() {
	var aj = ajax()
	aj.onreadystatechange=function() {
		if(xmlHttp.readyState==4) {
			while (txsre.rows.length>1)  txsre.deleteRow(1)
			txs = aj.responseXML.getElementsByTagName('tx')
			for (var i=0; i<txs.length; i++) {
				var c,row = txsre.insertRow(-1)

				row.className='hov'

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = (i+1).toString()

				c = row.insertCell(-1)
				c.className ='mono'
				c.innerHTML = xval(txs[i], 'id')

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = get_maturity(xval(txs[i], 'time'))

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = xval(txs[i], 'len')

				c=row.insertCell(-1);c.align='right'
				c.innerHTML = val2reason(xval(txs[i], 'reason'))
			}
			txsre.style.display = 'table'
		}
	}
	txs2s.style.display = txsre.style.display = 'none'
	xmlHttp.open("GET","txsre.xml", true);
	xmlHttp.send(null);
}
