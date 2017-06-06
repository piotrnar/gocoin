<?php

$page = preg_replace("/[^a-zA-Z_]/", "", $_GET['id']);
//echo $page;
if ($page=='') {
	$page = 'index';
}

function get_html($fn) {
	$content = file_get_contents($fn);

	$beg = strpos($content, "<body>");
	if ($beg!==FALSE) {
		$beg += 6;
	}
	$end = strpos($content, "</body>");
	if ($end===FALSE) {
		$end = strlen($content);
	}

	return substr($content, $beg, $end-$beg);
}

function get_title($content) {
	$beg = strpos($content, "<h1>");
	if ($beg!==FALSE) {
		$beg += 4;
	}
	$end = strpos($content, "</h1>");
	if ($end===FALSE) {
		$end = strlen($content);
	}

	return substr($content, $beg, $end-$beg);
}

$menu = get_html("menu.html");
$fielname = "gocoin_$page.html";

$apos = strpos($menu, 'href="' . $fielname);
if ($apos!==FALSE) {
	$menu = substr($menu, 0, $apos) . 'class="selected" ' . substr($menu, $apos);
}

$content = get_html($fielname);
$title = get_title($content);

// add class="selected" to the main menu


echo '<html>
<head>
<link rel="stylesheet" href="style.css" type="text/css">
<title>Gocoin: '.$title.'</title>
</head>
<body>
<table cellspacing="10">
<tr>
<td valign="top" width="200">';
echo $menu;
echo '</td>
<td valign="top">';
echo $content;
echo
'</td></tr></table>
</body>
</html>';

?>