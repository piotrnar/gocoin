# Description

Memory usage optimization for the "qdb" engine.


# Installation

Simply copy `membind.go` from this folder, one level up, overwriting the previous file.


# Known issues

This `membind.go` will only build if you have a gcc compiler properly installed.

It is known to cause a severe performance issues on VPS servers, specifically OpenVZ.
