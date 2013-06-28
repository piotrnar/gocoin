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
	try {
		return xml.getElementsByTagName(tag)[0].childNodes[0].nodeValue;
	} catch (e) {
		return NaN
	}
}


