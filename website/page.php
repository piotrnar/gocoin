<?php

$page = preg_replace("/[^a-zA-Z_]/", "", $_GET['id']);
//echo $page;
if ($page=='') {
	$page = 'index';
}

function get_between_tags($content, $tag) {
	$beg = strpos($content, "<$tag>");
	if ($beg!==FALSE) {
		$beg += strlen($tag)+2;
	}
	$end = strpos($content, "</$tag>");
	if ($end===FALSE) {
		$end = strlen($content);
	}
	return substr($content, $beg, $end-$beg);
}


function get_body($fn) {
	$content = file_get_contents($fn);
	return get_between_tags($content, "body");
}

$menu = get_body("menu.html");
$fielname = "gocoin_$page.html";

// add class="selected" to the main menu
$apos = strpos($menu, 'href="' . $fielname);
if ($apos!==FALSE) {
	$menu = substr($menu, 0, $apos) . 'class="selected" ' . substr($menu, $apos);
}

$content = get_body($fielname);
$title = get_between_tags($content, "h1");


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