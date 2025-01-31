/**
 * [js-sha256]{@link https://github.com/emn178/js-sha256}
 *
 * @version 0.6.0
 * @author Chen, Yi-Cyuan [emn178@gmail.com]
 * @copyright Chen, Yi-Cyuan 2014-2017
 * @license MIT
 */
/*jslint bitwise: true */
function ROL(t, r) {
    return new Number(t << r | t >>> 32 - r)
}

function F(t, r, e) {
    return new Number(t ^ r ^ e)
}

function G(t, r, e) {
    return new Number(t & r | ~t & e)
}

function H(t, r, e) {
    return new Number((t | ~r) ^ e)
}

function I(t, r, e) {
    return new Number(t & e | r & ~e)
}

function J(t, r, e) {
    return new Number(t ^ (r | ~e))
}

function mixOneRound(t, r, e, i, h, s, n, o) {
    switch (o) {
        case 0:
            t += F(r, e, i) + s + 0;
            break;
        case 1:
            t += G(r, e, i) + s + 1518500249;
            break;
        case 2:
            t += H(r, e, i) + s + 1859775393;
            break;
        case 3:
            t += I(r, e, i) + s + 2400959708;
            break;
        case 4:
            t += J(r, e, i) + s + 2840853838;
            break;
        case 5:
            t += J(r, e, i) + s + 1352829926;
            break;
        case 6:
            t += I(r, e, i) + s + 1548603684;
            break;
        case 7:
            t += H(r, e, i) + s + 1836072691;
            break;
        case 8:
            t += G(r, e, i) + s + 2053994217;
            break;
        case 9:
            t += F(r, e, i) + s + 0;
            break;
        default:
            document.write("Bogus round number")
    }
    t = ROL(t, n) + h, e = ROL(e, 10), t &= 4294967295, r &= 4294967295, e &= 4294967295, i &= 4294967295, h &= 4294967295;
    var a = new Array;
    return a[0] = t, a[1] = r, a[2] = e, a[3] = i, a[4] = h, a[5] = s, a[6] = n, a
}

function MDinit(t) {
    t[0] = 1732584193, t[1] = 4023233417, t[2] = 2562383102, t[3] = 271733878, t[4] = 3285377520
}

function compress(t, r) {
    blockA = new Array, blockB = new Array;
    for (var e, i = 0; i < 5; i++) blockA[i] = new Number(t[i]), blockB[i] = new Number(t[i]);
    for (var h = 0, s = 0; s < 5; s++)
        for (i = 0; i < 16; i++) e = mixOneRound(blockA[(h + 0) % 5], blockA[(h + 1) % 5], blockA[(h + 2) % 5], blockA[(h + 3) % 5], blockA[(h + 4) % 5], r[indexes[s][i]], ROLs[s][i], s), blockA[(h + 0) % 5] = e[0], blockA[(h + 1) % 5] = e[1], blockA[(h + 2) % 5] = e[2], blockA[(h + 3) % 5] = e[3], blockA[(h + 4) % 5] = e[4], h += 4;
    h = 0;
    for (s = 5; s < 10; s++)
        for (i = 0; i < 16; i++) e = mixOneRound(blockB[(h + 0) % 5], blockB[(h + 1) % 5], blockB[(h + 2) % 5], blockB[(h + 3) % 5], blockB[(h + 4) % 5], r[indexes[s][i]], ROLs[s][i], s), blockB[(h + 0) % 5] = e[0], blockB[(h + 1) % 5] = e[1], blockB[(h + 2) % 5] = e[2], blockB[(h + 3) % 5] = e[3], blockB[(h + 4) % 5] = e[4], h += 4;
    blockB[3] += blockA[2] + t[1], t[1] = t[2] + blockA[3] + blockB[4], t[2] = t[3] + blockA[4] + blockB[0], t[3] = t[4] + blockA[0] + blockB[1], t[4] = t[0] + blockA[1] + blockB[2], t[0] = blockB[3]
}

function zeroX(t) {
    for (var r = 0; r < 16; r++) t[r] = 0
}

function MDfinish(t, r, e, i) {
    var h = new Array(16);
    zeroX(h);
    for (var s = 0, n = 0; n < (63 & e); n++) h[n >>> 2] ^= (255 & r[s++]) << 8 * (3 & n);
    h[e >>> 2 & 15] ^= 1 << 8 * (3 & e) + 7, (63 & e) > 55 && (compress(t, h), zeroX(h = new Array(16))), h[14] = e << 3, h[15] = e >>> 29 | i << 3, compress(t, h)
}

function BYTES_TO_DWORD(t) {
    var r = (255 & t.charCodeAt(3)) << 24;
    return r |= (255 & t.charCodeAt(2)) << 16, r |= (255 & t.charCodeAt(1)) << 8, r |= 255 & t.charCodeAt(0)
}

function RMD(t) {
    var r, e = new Array(RMDsize / 32),
        i = new Array(RMDsize / 8);
    MDinit(e), r = t.length;
    var h = new Array(16);
    zeroX(h);
    for (var s = 0, n = r; n > 63; n -= 64) {
        for (o = 0; o < 16; o++) h[o] = BYTES_TO_DWORD(t.slice(s, s + 4)), s += 4;
        compress(e, h)
    }
    MDfinish(e, t.slice(s, r), r, 0);
    for (var o = 0; o < RMDsize / 8; o += 4) i[o] = 255 & e[o >>> 2], i[o + 1] = e[o >>> 2] >>> 8 & 255, i[o + 2] = e[o >>> 2] >>> 16 & 255, i[o + 3] = e[o >>> 2] >>> 24 & 255;
    return i
}

function p2kh_to_witness_p2sh(t) {
    var r = Base58.decode(t);
    if (25 != r.length || (0 != r[0] && 111 != r[0])) return "";
    var e = new Uint8Array(22);
    e[0] = 0, e[1] = 20;
    for (s = 0; s < 20; s++) e[2 + s] = r[1 + s];
    var i = RMD(sha256.update(e).array()),
        h = new Uint8Array(25);
    h[0] = 5;
    for (var s = 0; s < 20; s++) h[1 + s] = i[s];
    var n = sha256.update(sha256.update(h.slice(0, 21)).array()).array();
    return h[21] = n[0], h[22] = n[1], h[23] = n[2], h[24] = n[3], Base58.encode(h)
}! function() {
    "use strict";

    function t(t, r) {
        r ? (d[0] = d[16] = d[1] = d[2] = d[3] = d[4] = d[5] = d[6] = d[7] = d[8] = d[9] = d[10] = d[11] = d[12] = d[13] = d[14] = d[15] = 0, this.blocks = d) : this.blocks = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], t ? (this.h0 = 3238371032, this.h1 = 914150663, this.h2 = 812702999, this.h3 = 4144912697, this.h4 = 4290775857, this.h5 = 1750603025, this.h6 = 1694076839, this.h7 = 3204075428) : (this.h0 = 1779033703, this.h1 = 3144134277, this.h2 = 1013904242, this.h3 = 2773480762, this.h4 = 1359893119, this.h5 = 2600822924, this.h6 = 528734635, this.h7 = 1541459225), this.block = this.start = this.bytes = 0, this.finalized = this.hashed = !1, this.first = !0, this.is224 = t
    }

    function r(r, h, s) {
        var n = "string" != typeof r;
        if (n) {
            if (null === r || void 0 === r) throw e;
            r.constructor === i.ArrayBuffer && (r = new Uint8Array(r))
        }
        f = r.length;
        if (n) {
            if ("number" != typeof f || !Array.isArray(r) && (!o || !ArrayBuffer.isView(r))) throw e
        } else {
            for (var a, c = [], f = r.length, u = 0, l = 0; l < f; ++l)(a = r.charCodeAt(l)) < 128 ? c[u++] = a : a < 2048 ? (c[u++] = 192 | a >> 6, c[u++] = 128 | 63 & a) : a < 55296 || a >= 57344 ? (c[u++] = 224 | a >> 12, c[u++] = 128 | a >> 6 & 63, c[u++] = 128 | 63 & a) : (a = 65536 + ((1023 & a) << 10 | 1023 & r.charCodeAt(++l)), c[u++] = 240 | a >> 18, c[u++] = 128 | a >> 12 & 63, c[u++] = 128 | a >> 6 & 63, c[u++] = 128 | 63 & a);
            r = c
        }
        r.length > 64 && (r = new t(h, !0).update(r).array());
        for (var d = [], y = [], l = 0; l < 64; ++l) {
            var p = r[l] || 0;
            d[l] = 92 ^ p, y[l] = 54 ^ p
        }
        t.call(this, h, s), this.update(y), this.oKeyPad = d, this.inner = !0, this.sharedMemory = s
    }
    var e = "input is invalid type",
        i = "object" == typeof window ? window : {},
        h = !i.JS_SHA256_NO_NODE_JS && "object" == typeof process && process.versions && process.versions.node;
    h && (i = global);
    var s = !i.JS_SHA256_NO_COMMON_JS && "object" == typeof module && module.exports,
        n = "function" == typeof define && define.amd,
        o = "undefined" != typeof ArrayBuffer,
        a = "0123456789abcdef".split(""),
        c = [-2147483648, 8388608, 32768, 128],
        f = [24, 16, 8, 0],
        u = [1116352408, 1899447441, 3049323471, 3921009573, 961987163, 1508970993, 2453635748, 2870763221, 3624381080, 310598401, 607225278, 1426881987, 1925078388, 2162078206, 2614888103, 3248222580, 3835390401, 4022224774, 264347078, 604807628, 770255983, 1249150122, 1555081692, 1996064986, 2554220882, 2821834349, 2952996808, 3210313671, 3336571891, 3584528711, 113926993, 338241895, 666307205, 773529912, 1294757372, 1396182291, 1695183700, 1986661051, 2177026350, 2456956037, 2730485921, 2820302411, 3259730800, 3345764771, 3516065817, 3600352804, 4094571909, 275423344, 430227734, 506948616, 659060556, 883997877, 958139571, 1322822218, 1537002063, 1747873779, 1955562222, 2024104815, 2227730452, 2361852424, 2428436474, 2756734187, 3204031479, 3329325298],
        l = ["hex", "array", "digest", "arrayBuffer"],
        d = [];
    !i.JS_SHA256_NO_NODE_JS && Array.isArray || (Array.isArray = function(t) {
        return "[object Array]" === Object.prototype.toString.call(t)
    });
    var y = function(r, e) {
            return function(i) {
                return new t(e, !0).update(i)[r]()
            }
        },
        p = function(r) {
            var e = y("hex", r);
            h && (e = b(e, r)), e.create = function() {
                return new t(r)
            }, e.update = function(t) {
                return e.create().update(t)
            };
            for (var i = 0; i < l.length; ++i) {
                var s = l[i];
                e[s] = y(s, r)
            }
            return e
        },
        b = function(t, r) {
            var i = require("crypto"),
                h = require("buffer").Buffer,
                s = r ? "sha224" : "sha256";
            return function(r) {
                if ("string" == typeof r) return i.createHash(s).update(r, "utf8").digest("hex");
                if (null === r || void 0 === r) throw e;
                return r.constructor === ArrayBuffer && (r = new Uint8Array(r)), Array.isArray(r) || ArrayBuffer.isView(r) || r.constructor === h ? i.createHash(s).update(new h(r)).digest("hex") : t(r)
            }
        },
        A = function(t, e) {
            return function(i, h) {
                return new r(i, e, !0).update(h)[t]()
            }
        },
        k = function(t) {
            var e = A("hex", t);
            e.create = function(e) {
                return new r(e, t)
            }, e.update = function(t, r) {
                return e.create(t).update(r)
            };
            for (var i = 0; i < l.length; ++i) {
                var h = l[i];
                e[h] = A(h, t)
            }
            return e
        };
    t.prototype.update = function(t) {
        if (!this.finalized) {
            var r = "string" != typeof t;
            if (r) {
                if (null === t || void 0 === t) throw e;
                t.constructor === i.ArrayBuffer && (t = new Uint8Array(t))
            }
            var h = t.length;
            if (r && ("number" != typeof h || !Array.isArray(t) && (!o || !ArrayBuffer.isView(t)))) throw e;
            for (var s, n, a = 0, c = this.blocks; a < h;) {
                if (this.hashed && (this.hashed = !1, c[0] = this.block, c[16] = c[1] = c[2] = c[3] = c[4] = c[5] = c[6] = c[7] = c[8] = c[9] = c[10] = c[11] = c[12] = c[13] = c[14] = c[15] = 0), r)
                    for (n = this.start; a < h && n < 64; ++a) c[n >> 2] |= t[a] << f[3 & n++];
                else
                    for (n = this.start; a < h && n < 64; ++a)(s = t.charCodeAt(a)) < 128 ? c[n >> 2] |= s << f[3 & n++] : s < 2048 ? (c[n >> 2] |= (192 | s >> 6) << f[3 & n++], c[n >> 2] |= (128 | 63 & s) << f[3 & n++]) : s < 55296 || s >= 57344 ? (c[n >> 2] |= (224 | s >> 12) << f[3 & n++], c[n >> 2] |= (128 | s >> 6 & 63) << f[3 & n++], c[n >> 2] |= (128 | 63 & s) << f[3 & n++]) : (s = 65536 + ((1023 & s) << 10 | 1023 & t.charCodeAt(++a)), c[n >> 2] |= (240 | s >> 18) << f[3 & n++], c[n >> 2] |= (128 | s >> 12 & 63) << f[3 & n++], c[n >> 2] |= (128 | s >> 6 & 63) << f[3 & n++], c[n >> 2] |= (128 | 63 & s) << f[3 & n++]);
                this.lastByteIndex = n, this.bytes += n - this.start, n >= 64 ? (this.block = c[16], this.start = n - 64, this.hash(), this.hashed = !0) : this.start = n
            }
            return this
        }
    }, t.prototype.finalize = function() {
        if (!this.finalized) {
            this.finalized = !0;
            var t = this.blocks,
                r = this.lastByteIndex;
            t[16] = this.block, t[r >> 2] |= c[3 & r], this.block = t[16], r >= 56 && (this.hashed || this.hash(), t[0] = this.block, t[16] = t[1] = t[2] = t[3] = t[4] = t[5] = t[6] = t[7] = t[8] = t[9] = t[10] = t[11] = t[12] = t[13] = t[14] = t[15] = 0), t[15] = this.bytes << 3, this.hash()
        }
    }, t.prototype.hash = function() {
        var t, r, e, i, h, s, n, o, a, c = this.h0,
            f = this.h1,
            l = this.h2,
            d = this.h3,
            y = this.h4,
            p = this.h5,
            b = this.h6,
            A = this.h7,
            k = this.blocks;
        for (t = 16; t < 64; ++t) r = ((h = k[t - 15]) >>> 7 | h << 25) ^ (h >>> 18 | h << 14) ^ h >>> 3, e = ((h = k[t - 2]) >>> 17 | h << 15) ^ (h >>> 19 | h << 13) ^ h >>> 10, k[t] = k[t - 16] + r + k[t - 7] + e << 0;
        for (a = f & l, t = 0; t < 64; t += 4) this.first ? (this.is224 ? (s = 300032, A = (h = k[0] - 1413257819) - 150054599 << 0, d = h + 24177077 << 0) : (s = 704751109, A = (h = k[0] - 210244248) - 1521486534 << 0, d = h + 143694565 << 0), this.first = !1) : (r = (c >>> 2 | c << 30) ^ (c >>> 13 | c << 19) ^ (c >>> 22 | c << 10), i = (s = c & f) ^ c & l ^ a, A = d + (h = A + (e = (y >>> 6 | y << 26) ^ (y >>> 11 | y << 21) ^ (y >>> 25 | y << 7)) + (y & p ^ ~y & b) + u[t] + k[t]) << 0, d = h + (r + i) << 0), r = (d >>> 2 | d << 30) ^ (d >>> 13 | d << 19) ^ (d >>> 22 | d << 10), i = (n = d & c) ^ d & f ^ s, b = l + (h = b + (e = (A >>> 6 | A << 26) ^ (A >>> 11 | A << 21) ^ (A >>> 25 | A << 7)) + (A & y ^ ~A & p) + u[t + 1] + k[t + 1]) << 0, r = ((l = h + (r + i) << 0) >>> 2 | l << 30) ^ (l >>> 13 | l << 19) ^ (l >>> 22 | l << 10), i = (o = l & d) ^ l & c ^ n, p = f + (h = p + (e = (b >>> 6 | b << 26) ^ (b >>> 11 | b << 21) ^ (b >>> 25 | b << 7)) + (b & A ^ ~b & y) + u[t + 2] + k[t + 2]) << 0, r = ((f = h + (r + i) << 0) >>> 2 | f << 30) ^ (f >>> 13 | f << 19) ^ (f >>> 22 | f << 10), i = (a = f & l) ^ f & d ^ o, y = c + (h = y + (e = (p >>> 6 | p << 26) ^ (p >>> 11 | p << 21) ^ (p >>> 25 | p << 7)) + (p & b ^ ~p & A) + u[t + 3] + k[t + 3]) << 0, c = h + (r + i) << 0;
        this.h0 = this.h0 + c << 0, this.h1 = this.h1 + f << 0, this.h2 = this.h2 + l << 0, this.h3 = this.h3 + d << 0, this.h4 = this.h4 + y << 0, this.h5 = this.h5 + p << 0, this.h6 = this.h6 + b << 0, this.h7 = this.h7 + A << 0
    }, t.prototype.hex = function() {
        this.finalize();
        var t = this.h0,
            r = this.h1,
            e = this.h2,
            i = this.h3,
            h = this.h4,
            s = this.h5,
            n = this.h6,
            o = this.h7,
            c = a[t >> 28 & 15] + a[t >> 24 & 15] + a[t >> 20 & 15] + a[t >> 16 & 15] + a[t >> 12 & 15] + a[t >> 8 & 15] + a[t >> 4 & 15] + a[15 & t] + a[r >> 28 & 15] + a[r >> 24 & 15] + a[r >> 20 & 15] + a[r >> 16 & 15] + a[r >> 12 & 15] + a[r >> 8 & 15] + a[r >> 4 & 15] + a[15 & r] + a[e >> 28 & 15] + a[e >> 24 & 15] + a[e >> 20 & 15] + a[e >> 16 & 15] + a[e >> 12 & 15] + a[e >> 8 & 15] + a[e >> 4 & 15] + a[15 & e] + a[i >> 28 & 15] + a[i >> 24 & 15] + a[i >> 20 & 15] + a[i >> 16 & 15] + a[i >> 12 & 15] + a[i >> 8 & 15] + a[i >> 4 & 15] + a[15 & i] + a[h >> 28 & 15] + a[h >> 24 & 15] + a[h >> 20 & 15] + a[h >> 16 & 15] + a[h >> 12 & 15] + a[h >> 8 & 15] + a[h >> 4 & 15] + a[15 & h] + a[s >> 28 & 15] + a[s >> 24 & 15] + a[s >> 20 & 15] + a[s >> 16 & 15] + a[s >> 12 & 15] + a[s >> 8 & 15] + a[s >> 4 & 15] + a[15 & s] + a[n >> 28 & 15] + a[n >> 24 & 15] + a[n >> 20 & 15] + a[n >> 16 & 15] + a[n >> 12 & 15] + a[n >> 8 & 15] + a[n >> 4 & 15] + a[15 & n];
        return this.is224 || (c += a[o >> 28 & 15] + a[o >> 24 & 15] + a[o >> 20 & 15] + a[o >> 16 & 15] + a[o >> 12 & 15] + a[o >> 8 & 15] + a[o >> 4 & 15] + a[15 & o]), c
    }, t.prototype.toString = t.prototype.hex, t.prototype.digest = function() {
        this.finalize();
        var t = this.h0,
            r = this.h1,
            e = this.h2,
            i = this.h3,
            h = this.h4,
            s = this.h5,
            n = this.h6,
            o = this.h7,
            a = [t >> 24 & 255, t >> 16 & 255, t >> 8 & 255, 255 & t, r >> 24 & 255, r >> 16 & 255, r >> 8 & 255, 255 & r, e >> 24 & 255, e >> 16 & 255, e >> 8 & 255, 255 & e, i >> 24 & 255, i >> 16 & 255, i >> 8 & 255, 255 & i, h >> 24 & 255, h >> 16 & 255, h >> 8 & 255, 255 & h, s >> 24 & 255, s >> 16 & 255, s >> 8 & 255, 255 & s, n >> 24 & 255, n >> 16 & 255, n >> 8 & 255, 255 & n];
        return this.is224 || a.push(o >> 24 & 255, o >> 16 & 255, o >> 8 & 255, 255 & o), a
    }, t.prototype.array = t.prototype.digest, t.prototype.arrayBuffer = function() {
        this.finalize();
        var t = new ArrayBuffer(this.is224 ? 28 : 32),
            r = new DataView(t);
        return r.setUint32(0, this.h0), r.setUint32(4, this.h1), r.setUint32(8, this.h2), r.setUint32(12, this.h3), r.setUint32(16, this.h4), r.setUint32(20, this.h5), r.setUint32(24, this.h6), this.is224 || r.setUint32(28, this.h7), t
    }, (r.prototype = new t).finalize = function() {
        if (t.prototype.finalize.call(this), this.inner) {
            this.inner = !1;
            var r = this.array();
            t.call(this, this.is224, this.sharedMemory), this.update(this.oKeyPad), this.update(r), t.prototype.finalize.call(this)
        }
    };
    var v = p();
    v.sha256 = v, v.sha224 = p(!0), v.sha256.hmac = k(), v.sha224.hmac = k(!0), s ? module.exports = v : (i.sha256 = v.sha256, i.sha224 = v.sha224, n && define(function() {
        return v
    }))
}();
var RMDsize = 160,
    X = new Array,
    ROLs = [
        [11, 14, 15, 12, 5, 8, 7, 9, 11, 13, 14, 15, 6, 7, 9, 8],
        [7, 6, 8, 13, 11, 9, 7, 15, 7, 12, 15, 9, 11, 7, 13, 12],
        [11, 13, 6, 7, 14, 9, 13, 15, 14, 8, 13, 6, 5, 12, 7, 5],
        [11, 12, 14, 15, 14, 15, 9, 8, 9, 14, 5, 6, 8, 6, 5, 12],
        [9, 15, 5, 11, 6, 8, 13, 12, 5, 12, 13, 14, 11, 8, 5, 6],
        [8, 9, 9, 11, 13, 15, 15, 5, 7, 7, 8, 11, 14, 14, 12, 6],
        [9, 13, 15, 7, 12, 8, 9, 11, 7, 7, 12, 7, 6, 15, 13, 11],
        [9, 7, 15, 11, 8, 6, 6, 14, 12, 13, 5, 14, 13, 13, 7, 5],
        [15, 5, 8, 11, 14, 14, 6, 14, 6, 9, 12, 9, 12, 5, 15, 8],
        [8, 5, 12, 9, 12, 5, 14, 6, 8, 13, 6, 5, 15, 13, 11, 11]
    ],
    indexes = [
        [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15],
        [7, 4, 13, 1, 10, 6, 15, 3, 12, 0, 9, 5, 2, 14, 11, 8],
        [3, 10, 14, 4, 9, 15, 8, 1, 2, 7, 0, 6, 13, 11, 5, 12],
        [1, 9, 11, 10, 0, 8, 12, 4, 13, 3, 7, 15, 14, 5, 6, 2],
        [4, 0, 5, 9, 7, 12, 2, 10, 14, 1, 3, 8, 11, 6, 15, 13],
        [5, 14, 7, 0, 9, 2, 11, 4, 13, 6, 15, 8, 1, 10, 3, 12],
        [6, 11, 3, 7, 0, 13, 5, 10, 14, 15, 8, 12, 4, 9, 1, 2],
        [15, 5, 1, 3, 7, 14, 6, 9, 11, 8, 12, 2, 10, 0, 4, 13],
        [8, 6, 4, 1, 3, 11, 15, 0, 5, 12, 2, 13, 9, 7, 10, 14],
        [12, 15, 10, 4, 1, 5, 8, 7, 6, 2, 13, 14, 0, 3, 9, 11]
    ];
(function() {
    var t, r, e, i;
    for (e = ("undefined" != typeof module && null !== module ? module.exports : void 0) || (window.Base58 = {}), t = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", r = {}, i = 0; i < t.length;) r[t.charAt(i)] = i, i++;
    e.encode = function(r) {
        var e, h, s;
        if (0 === r.length) return "";
        for (i = void 0, s = void 0, h = [0], i = 0; i < r.length;) {
            for (s = 0; s < h.length;) h[s] <<= 8, s++;
            for (h[0] += r[i], e = 0, s = 0; s < h.length;) h[s] += e, e = h[s] / 58 | 0, h[s] %= 58, ++s;
            for (; e;) h.push(e % 58), e = e / 58 | 0;
            i++
        }
        for (i = 0; 0 === r[i] && i < r.length - 1;) h.push(0), i++;
        return h.reverse().map(function(r) {
            return t[r]
        }).join("")
    }, e.decode = function(t) {
        var e, h, s, n;
        if (0 === t.length) return new("undefined" != typeof Uint8Array && null !== Uint8Array ? Uint8Array : Buffer)(0);
        for (i = void 0, n = void 0, e = [0], i = 0; i < t.length;) {
            if (!((h = t[i]) in r)) throw "Base58.decode received unacceptable input. Character '" + h + "' is not in the Base58 alphabet.";
            for (n = 0; n < e.length;) e[n] *= 58, n++;
            for (e[0] += r[h], s = 0, n = 0; n < e.length;) e[n] += s, s = e[n] >> 8, e[n] &= 255, ++n;
            for (; s;) e.push(255 & s), s >>= 8;
            i++
        }
        for (i = 0;
            "1" === t[i] && i < t.length - 1;) e.push(0), i++;
        return new("undefined" != typeof Uint8Array && null !== Uint8Array ? Uint8Array : Buffer)(e.reverse())
    }
}).call(this)

function pubkey_to_p2kh(pkb) {
	var i = RMD(sha256.update(pkb).array())
	var h = new Uint8Array(25)
	h[0] = testnet ? 111 : 0
	for (var s = 0; s < 20; s++) h[1 + s] = i[s]
	var n = sha256.update(sha256.update(h.slice(0, 21)).array()).array()
	h[21] = n[0]
	h[22] = n[1]
	h[23] = n[2]
	h[24] = n[3]
	return Base58.encode(h)
}

