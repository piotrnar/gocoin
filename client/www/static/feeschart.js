document.write(`
<div id="light" class="white_content" style="height:auto">
<div id="block_fee_stats" width="100%" style="text-align:center">

<div style="margin-top:10px">
Block number <b id="stat_height"></b> /
<b id="stat_block_size"></b> bytes
-
Max <b id="stat_max_fee"></b> SPB
&nbsp;&bull;&nbsp;
Avg <b id="stat_avg_fee"></b> SPB
&nbsp;&bull;&nbsp;
Min <b id="stat_min_fee"></b> SPB
&nbsp;&bull;&nbsp;
Mined by <b id="stat_mined_by"></b>
<span style="float:right"><img title="Close this popup" src="static/close.png" class="hand" onclick="closepopup()">&nbsp;</span>
<br><br>
</div>

<div id="stat_error" class="err" style="display:none">Something went wrong (<span id="error_info"></span>)</div>
<div id="block_fees" style="height:370px;margin:5px"></div>
<br>
<div width="100%" style="margin-bottom:10px;text-align:right">
<span class="hand" onclick="block_fees_points.click()">
	<input type="checkbox" id="block_fees_points" onchange="show_fees_clicked()" onclick="event.stopPropagation()"> Show points
</span>
&nbsp;&bull;&nbsp;
Limit range to
<span class="hand" onclick="block_fees_full.click()">
    <input type="radio" name="block_fees_range" id="block_fees_full" onchange="show_fees_clicked()" onclick="event.stopPropagation()">100%
</span>
<span class="hand" onclick="block_fees_25.click()">
	<input type="radio" name="block_fees_range" id="block_fees_25" onchange="show_fees_clicked()" onclick="event.stopPropagation()" checked>25%
</span>
<span class="hand" onclick="block_fees_5.click()">
	<input type="radio" name="block_fees_range" id="block_fees_5" onchange="show_fees_clicked()" onclick="event.stopPropagation()">5%
</span>
&nbsp;&bull;&nbsp;
<span class="hand" onclick="block_fees_raw.click()">
	<input type="radio" name="block_fees_mode" id="block_fees_raw" value="raw" onchange="show_fees_clicked()" onclick="event.stopPropagation()" checked> Show as is
</span>
&nbsp;&bull;&nbsp;
<span class="hand" onclick="block_fees_gru.click()">
	<input type="radio" name="block_fees_mode" id="block_fees_gru" value="gru" onchange="show_fees_clicked()" onclick="event.stopPropagation()"> Try to group
</span>
&nbsp;&bull;&nbsp;
<span class="hand" onclick="block_fees_spb.click()">
	<input type="radio" name="block_fees_mode" id="block_fees_spb" value="spb" onchange="show_fees_clicked()" onclick="event.stopPropagation()"> Sort by SPB
</span>
</div>

</div>
</div><div id="fade" class="black_overlay"></div>
`)

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
		'background-color': dark_mode ? '#202020' : '#c0c0c0',
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
					points: { show:block_fees_points.checked, fillColor:flot_points_fill_color() },
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



function fees_chart_restore_settings() {
	if (localStorage.getItem("fees_chart_points")) block_fees_points.checked = true
	var val = localStorage.getItem("fees_chart_scale")
	if (val==100) block_fees_full.checked=true
	else if (val==5) block_fees_5.checked=true
	val = localStorage.getItem("fees_chart_mode")
	if (val=="group")  block_fees_gru.checked=true
	else  if (val=="sort")  block_fees_spb.checked=true
}
fees_chart_restore_settings()
