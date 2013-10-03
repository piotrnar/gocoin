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
