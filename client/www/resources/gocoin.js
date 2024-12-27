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

function noscroll() {
	scroll(0,0)
}

function closepopup_x(fees) {
	if (light.style.display!='none') {
		if (fees) {
			$("#block_fees").unbind("plothover")
			$("#fees_tooltip").remove()
		}
		light.style.display='none'
		fade.style.display='none'
		window.scrollTo(0,prvpos)
		document.removeEventListener("scroll", noscroll)
	}
}

function closepopup() {
	closepopup_x(true)
}

function css(selector, property, value) {
	for (var i=0; i<document.styleSheets.length;i++) {//Loop through all styles
		//Try add rule
		try {
			document.styleSheets[i].insertRule(selector+ ' {'+property+':'+value+'}', document.styleSheets[i].cssRules.length);
		} catch(err) {try { document.styleSheets[i].addRule(selector, property+':'+value);} catch(err) {}}//IE
	}
}


var last_height // used for showing block fees
var fees_plot_data = [ { data : [] } ];
var previousPoint = null
var max_spb, totbytes

function show_fees_tooltip(x, y, contents) {
	$('<div id="fees_tooltip">' + contents + '</div>').css( {
		'position': 'absolute',
		'display': 'none',
		'top': y - 29,
		'left': x + 4,
		'text-align' : 'center',
		'z-index': 9999,
		'border': '2px solid green',
		'padding': '5px',
		'font-size' : '12px',
		'background-color': '#c0c0c0',
		'opacity': 1
	}).appendTo("body").fadeIn(200);
}

function show_cursor_tooltip(x, y, contents) {
	$('<div id="fees_tooltip">' + contents + '</div>').css( {
		'position': 'absolute',
		'display': 'none',
		'top': y - 29,
		'left': x + 4,
		'text-align' : 'center',
		'border': '1px solid black',
		'z-index': 9999,
		'padding': '3px',
		'font-size' : '12px',
		'background-color': 'white',
		'color': 'rgba(170, 0, 0, 0.80)',
		'opacity': 1
	}).appendTo("body").fadeIn(200);
}

function show_fees_handlehover(event, pos, item) {
	if (item) {
		if (previousPoint != item.dataIndex) {
			var rec = fees_plot_data[0].data[item.dataIndex]
			previousPoint = item.dataIndex
			$("#fees_tooltip").remove()
			var str = '<b>' + parseFloat(rec[1]).toFixed(2) + '</b> SPB at ' +
				'<b>' + (100.0*rec[0]/1e6).toFixed(0) + '%</b><br>(of max block size)'
			show_fees_tooltip(item.pageX, item.pageY, str)
		}
	} else {
		$("#fees_tooltip").remove()
		var x = pos.x
		var y = pos.y
		var show_x = (x >= 0 && x<=totbytes)
		var show_y = (y >= 0 && y<=max_spb)

		var str = ''
		if (show_y)  str += '<b>' + parseFloat(y).toFixed(2) + '</b> SPB'
		if (show_x) {
			if (str!='')  str += '  |  '
			str += '<b>' + (100.0*x/1e6).toFixed(0) + '%</b>'
		}
		if (str!='') {
			show_cursor_tooltip(pos.pageX, pos.pageY, str)
		}

		previousPoint = null
	}
}

function show_fees_clicked(height) {
	var aj = ajax()
	aj.onreadystatechange=function() {
		if(aj.readyState==4) {
			if (prvpos==null) {
				fade.addEventListener('click', closepopup)
				fade.style.cursor = 'pointer'
				fade.title = 'Click here to close the popup'
			}

			prvpos = document.body.scrollTop
			window.scrollTo(0,0)

			light.style.display='block'
			fade.style.display='block'
			document.addEventListener("scroll", noscroll)


			try {
				var showblfees_stats = JSON.parse(aj.responseText)

				// hide error message
				stat_error.style.display = 'none'

				if (block_fees_gru.checked) {
					var stat2 = new Array()
					var curr_elem = [0,0,0]
					for (var i=0; i<showblfees_stats.length; i++) {
						if (showblfees_stats[i][2]!=curr_elem[2]) {
							if (curr_elem[0] > 0) {
								stat2.push(curr_elem)
							}
							curr_elem = [0,0,showblfees_stats[i][2]]
						}
						curr_elem[0] += showblfees_stats[i][0]
						curr_elem[1] += showblfees_stats[i][1]
					}
					if (curr_elem[0] > 0) {
						stat2.push(curr_elem)
					}
					showblfees_stats = stat2
					localStorage.setItem("fees_chart_mode", "group")
				} else if (block_fees_spb.checked) {
					showblfees_stats.sort(function (a, b) {
						var spb_a = a[1] / a[0]
						var spb_b = b[1] / b[0]
						return ( spb_a < spb_b ) ? 1 : ( (spb_a != spb_b) ? -1 : 0 )
					})
					localStorage.setItem("fees_chart_mode", "sort")
				} else {
					localStorage.setItem("fees_chart_mode", "asis")
				}

				fees_plot_data = [ { data : [] } ];

				localStorage.setItem("fees_chart_points", block_fees_points.checked)
				var plot_options = {
					xaxis: { position : "top", alignTicksWithAxis: 200 },
					yaxis : { position : "right", tickFormatter : function(a) {return a.toFixed((a>9||a==0)?0:(a>0.9?1:2)) + " SPB"}, labelWidth : 60 },
					crosshair : {mode: "xy"},
					grid: { hoverable: true, clickable: false },
					points: { show:block_fees_points.checked },
					lines: {show:true, fill:true}
				}

				var min_spb, spb
				totbytes = 0
				var totfees = 0
				for (var i=0; i<showblfees_stats.length; i++) {
					spb = showblfees_stats[i][1] / (showblfees_stats[i][0] / 4)
					if (i==0) {
						max_spb = min_spb = spb
					} else {
						if (spb>max_spb) max_spb=spb
						else if (spb<min_spb) min_spb=spb
					}

					totbytes += (showblfees_stats[i][0] / 4)
					totfees += showblfees_stats[i][1]
					fees_plot_data[0].data.push([totbytes, spb])
				}

				var avg_fee = totfees / totbytes
				stat_max_fee.innerText = max_spb.toFixed(0)
				stat_avg_fee.innerText = avg_fee.toFixed(1)
				stat_min_fee.innerText = min_spb.toFixed(2)

				if (!block_fees_full.checked) {
					if (block_fees_25.checked) {
						max_spb /= 4
						localStorage.setItem("fees_chart_scale", 25)
					} else  if (block_fees_5.checked) {
						max_spb /= 20
						localStorage.setItem("fees_chart_scale", 5)
					}
					plot_options.yaxis.max = max_spb
				} else {
					localStorage.setItem("fees_chart_scale", 100)
				}


				$.plot($("#block_fees"), fees_plot_data, plot_options)

				$("#block_fees").unbind("plothover")
				$("#block_fees").bind("plothover", show_fees_handlehover)
			} catch (e) {
				console.log('error', e)
				error_info.innerText = aj.responseText
				stat_error.style.display = 'block'
				$("#block_fees").empty()
			}
		}
	}
	aj.open("GET","blfees.json?height="+last_height, true);
	aj.send(null);
}

function show_block_fees(height,size,minedby) {
	last_height = height // for refreshing the chart
	stat_height.innerText = height
	stat_block_size.innerText = size
	stat_mined_by.innerText = minedby
	show_fees_clicked()
}
