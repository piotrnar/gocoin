// Copyright (c) 2013 Pieter Wuille
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

#include <stdio.h>
#include <stdlib.h>
#include <assert.h>
#include <string.h>
#include "num.h"
#include "field.h"

const secp256k1_fe_consts_t *secp256k1_fe_consts = NULL;

void secp256k1_fe_inner_start(void) {}
void secp256k1_fe_inner_stop(void) {}

void secp256k1_fe_normalize(secp256k1_fe_t *r) {
//    fog("normalize in: ", r);
    uint32_t c;
    c = r->n[0];
    uint32_t t0 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[1];
    uint32_t t1 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[2];
    uint32_t t2 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[3];
    uint32_t t3 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[4];
    uint32_t t4 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[5];
    uint32_t t5 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[6];
    uint32_t t6 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[7];
    uint32_t t7 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[8];
    uint32_t t8 = c & 0x3FFFFFFUL;
    c = (c >> 26) + r->n[9];
    uint32_t t9 = c & 0x03FFFFFUL;
    c >>= 22;
/*    r->n[0] = t0; r->n[1] = t1; r->n[2] = t2; r->n[3] = t3; r->n[4] = t4;
    r->n[5] = t5; r->n[6] = t6; r->n[7] = t7; r->n[8] = t8; r->n[9] = t9;
    fog("         tm1: ", r);
    fprintf(stderr, "out c= %08lx\n", (unsigned long)c);*/

    // The following code will not modify the t's if c is initially 0.
    uint32_t d = c * 0x3D1UL + t0;
    t0 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t1 + c*0x40;
    t1 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t2;
    t2 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t3;
    t3 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t4;
    t4 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t5;
    t5 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t6;
    t6 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t7;
    t7 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t8;
    t8 = d & 0x3FFFFFFULL;
    d = (d >> 26) + t9;
    t9 = d & 0x03FFFFFULL;
    assert((d >> 22) == 0);
/*    r->n[0] = t0; r->n[1] = t1; r->n[2] = t2; r->n[3] = t3; r->n[4] = t4;
    r->n[5] = t5; r->n[6] = t6; r->n[7] = t7; r->n[8] = t8; r->n[9] = t9;
    fog("         tm2: ", r); */

    // Subtract p if result >= p
    uint64_t low = ((uint64_t)t1 << 26) | t0;
    uint64_t mask = -(int64_t)((t9 < 0x03FFFFFUL) | (t8 < 0x3FFFFFFUL) | (t7 < 0x3FFFFFFUL) | (t6 < 0x3FFFFFFUL) | (t5 < 0x3FFFFFFUL) | (t4 < 0x3FFFFFFUL) | (t3 < 0x3FFFFFFUL) | (t2 < 0x3FFFFFFUL) | (low < 0xFFFFEFFFFFC2FULL));
    t9 &= mask;
    t8 &= mask;
    t7 &= mask;
    t6 &= mask;
    t5 &= mask;
    t4 &= mask;
    t3 &= mask;
    t2 &= mask;
    low -= (~mask & 0xFFFFEFFFFFC2FULL);

    // push internal variables back
    r->n[0] = low & 0x3FFFFFFUL; r->n[1] = (low >> 26) & 0x3FFFFFFUL; r->n[2] = t2; r->n[3] = t3; r->n[4] = t4;
    r->n[5] = t5; r->n[6] = t6; r->n[7] = t7; r->n[8] = t8; r->n[9] = t9;
/*    fog("         out: ", r);*/

#ifdef VERIFY
    r->magnitude = 1;
    r->normalized = 1;
#endif
}

void inline secp256k1_fe_set_int(secp256k1_fe_t *r, int a) {
    r->n[0] = a;
    r->n[1] = r->n[2] = r->n[3] = r->n[4] = r->n[5] = r->n[6] = r->n[7] = r->n[8] = r->n[9] = 0;
#ifdef VERIFY
    r->magnitude = 1;
    r->normalized = 1;
#endif
}

// TODO: not constant time!
int inline secp256k1_fe_is_zero(const secp256k1_fe_t *a) {
#ifdef VERIFY
    assert(a->normalized);
#endif
    return (a->n[0] == 0 && a->n[1] == 0 && a->n[2] == 0 && a->n[3] == 0 && a->n[4] == 0 && a->n[5] == 0 && a->n[6] == 0 && a->n[7] == 0 && a->n[8] == 0 && a->n[9] == 0);
}

int inline secp256k1_fe_is_odd(const secp256k1_fe_t *a) {
#ifdef VERIFY
    assert(a->normalized);
#endif
    return a->n[0] & 1;
}

// TODO: not constant time!
int inline secp256k1_fe_equal(const secp256k1_fe_t *a, const secp256k1_fe_t *b) {
#ifdef VERIFY
    assert(a->normalized);
    assert(b->normalized);
#endif
    return (a->n[0] == b->n[0] && a->n[1] == b->n[1] && a->n[2] == b->n[2] && a->n[3] == b->n[3] && a->n[4] == b->n[4] &&
            a->n[5] == b->n[5] && a->n[6] == b->n[6] && a->n[7] == b->n[7] && a->n[8] == b->n[8] && a->n[9] == b->n[9]);
}

void secp256k1_fe_set_b32(secp256k1_fe_t *r, const unsigned char *a) {
    r->n[0] = r->n[1] = r->n[2] = r->n[3] = r->n[4] = 0;
    r->n[5] = r->n[6] = r->n[7] = r->n[8] = r->n[9] = 0;
    for (int i=0; i<32; i++) {
        for (int j=0; j<4; j++) {
            int limb = (8*i+2*j)/26;
            int shift = (8*i+2*j)%26;
            r->n[limb] |= (uint32_t)((a[31-i] >> (2*j)) & 0x3) << shift;
        }
    }
#ifdef VERIFY
    r->magnitude = 1;
    r->normalized = 1;
#endif
}

/** Convert a field element to a 32-byte big endian value. Requires the input to be normalized */
void secp256k1_fe_get_b32(unsigned char *r, const secp256k1_fe_t *a) {
#ifdef VERIFY
    assert(a->normalized);
#endif
    for (int i=0; i<32; i++) {
        int c = 0;
        for (int j=0; j<4; j++) {
            int limb = (8*i+2*j)/26;
            int shift = (8*i+2*j)%26;
            c |= ((a->n[limb] >> shift) & 0x3) << (2 * j);
        }
        r[31-i] = c;
    }
}

void inline secp256k1_fe_negate(secp256k1_fe_t *r, const secp256k1_fe_t *a, int m) {
#ifdef VERIFY
    assert(a->magnitude <= m);
    r->magnitude = m + 1;
    r->normalized = 0;
#endif
    r->n[0] = 0x3FFFC2FUL * (m + 1) - a->n[0];
    r->n[1] = 0x3FFFFBFUL * (m + 1) - a->n[1];
    r->n[2] = 0x3FFFFFFUL * (m + 1) - a->n[2];
    r->n[3] = 0x3FFFFFFUL * (m + 1) - a->n[3];
    r->n[4] = 0x3FFFFFFUL * (m + 1) - a->n[4];
    r->n[5] = 0x3FFFFFFUL * (m + 1) - a->n[5];
    r->n[6] = 0x3FFFFFFUL * (m + 1) - a->n[6];
    r->n[7] = 0x3FFFFFFUL * (m + 1) - a->n[7];
    r->n[8] = 0x3FFFFFFUL * (m + 1) - a->n[8];
    r->n[9] = 0x03FFFFFUL * (m + 1) - a->n[9];
}

void inline secp256k1_fe_mul_int(secp256k1_fe_t *r, int a) {
#ifdef VERIFY
    r->magnitude *= a;
    r->normalized = 0;
#endif
    r->n[0] *= a;
    r->n[1] *= a;
    r->n[2] *= a;
    r->n[3] *= a;
    r->n[4] *= a;
    r->n[5] *= a;
    r->n[6] *= a;
    r->n[7] *= a;
    r->n[8] *= a;
    r->n[9] *= a;
}

void inline secp256k1_fe_add(secp256k1_fe_t *r, const secp256k1_fe_t *a) {
#ifdef VERIFY
    r->magnitude += a->magnitude;
    r->normalized = 0;
#endif
    r->n[0] += a->n[0];
    r->n[1] += a->n[1];
    r->n[2] += a->n[2];
    r->n[3] += a->n[3];
    r->n[4] += a->n[4];
    r->n[5] += a->n[5];
    r->n[6] += a->n[6];
    r->n[7] += a->n[7];
    r->n[8] += a->n[8];
    r->n[9] += a->n[9];
}

void inline secp256k1_fe_mul_inner(const uint32_t *a, const uint32_t *b, uint32_t *r) {
    uint64_t c = (uint64_t)a[0] * b[0];
    uint32_t t0 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[1] +
            (uint64_t)a[1] * b[0];
    uint32_t t1 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[2] +
            (uint64_t)a[1] * b[1] +
            (uint64_t)a[2] * b[0];
    uint32_t t2 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[3] +
            (uint64_t)a[1] * b[2] +
            (uint64_t)a[2] * b[1] +
            (uint64_t)a[3] * b[0];
    uint32_t t3 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[4] +
            (uint64_t)a[1] * b[3] +
            (uint64_t)a[2] * b[2] +
            (uint64_t)a[3] * b[1] +
            (uint64_t)a[4] * b[0];
    uint32_t t4 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[5] +
            (uint64_t)a[1] * b[4] +
            (uint64_t)a[2] * b[3] +
            (uint64_t)a[3] * b[2] +
            (uint64_t)a[4] * b[1] +
            (uint64_t)a[5] * b[0];
    uint32_t t5 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[6] +
            (uint64_t)a[1] * b[5] +
            (uint64_t)a[2] * b[4] +
            (uint64_t)a[3] * b[3] +
            (uint64_t)a[4] * b[2] +
            (uint64_t)a[5] * b[1] +
            (uint64_t)a[6] * b[0];
    uint32_t t6 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[7] +
            (uint64_t)a[1] * b[6] +
            (uint64_t)a[2] * b[5] +
            (uint64_t)a[3] * b[4] +
            (uint64_t)a[4] * b[3] +
            (uint64_t)a[5] * b[2] +
            (uint64_t)a[6] * b[1] +
            (uint64_t)a[7] * b[0];
    uint32_t t7 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[8] +
            (uint64_t)a[1] * b[7] +
            (uint64_t)a[2] * b[6] +
            (uint64_t)a[3] * b[5] +
            (uint64_t)a[4] * b[4] +
            (uint64_t)a[5] * b[3] +
            (uint64_t)a[6] * b[2] +
            (uint64_t)a[7] * b[1] +
            (uint64_t)a[8] * b[0];
    uint32_t t8 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[0] * b[9] +
            (uint64_t)a[1] * b[8] +
            (uint64_t)a[2] * b[7] +
            (uint64_t)a[3] * b[6] +
            (uint64_t)a[4] * b[5] +
            (uint64_t)a[5] * b[4] +
            (uint64_t)a[6] * b[3] +
            (uint64_t)a[7] * b[2] +
            (uint64_t)a[8] * b[1] +
            (uint64_t)a[9] * b[0];
    uint32_t t9 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[1] * b[9] +
            (uint64_t)a[2] * b[8] +
            (uint64_t)a[3] * b[7] +
            (uint64_t)a[4] * b[6] +
            (uint64_t)a[5] * b[5] +
            (uint64_t)a[6] * b[4] +
            (uint64_t)a[7] * b[3] +
            (uint64_t)a[8] * b[2] +
            (uint64_t)a[9] * b[1];
    uint32_t t10 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[2] * b[9] +
            (uint64_t)a[3] * b[8] +
            (uint64_t)a[4] * b[7] +
            (uint64_t)a[5] * b[6] +
            (uint64_t)a[6] * b[5] +
            (uint64_t)a[7] * b[4] +
            (uint64_t)a[8] * b[3] +
            (uint64_t)a[9] * b[2];
    uint32_t t11 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[3] * b[9] +
            (uint64_t)a[4] * b[8] +
            (uint64_t)a[5] * b[7] +
            (uint64_t)a[6] * b[6] +
            (uint64_t)a[7] * b[5] +
            (uint64_t)a[8] * b[4] +
            (uint64_t)a[9] * b[3];
    uint32_t t12 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[4] * b[9] +
            (uint64_t)a[5] * b[8] +
            (uint64_t)a[6] * b[7] +
            (uint64_t)a[7] * b[6] +
            (uint64_t)a[8] * b[5] +
            (uint64_t)a[9] * b[4];
    uint32_t t13 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[5] * b[9] +
            (uint64_t)a[6] * b[8] +
            (uint64_t)a[7] * b[7] +
            (uint64_t)a[8] * b[6] +
            (uint64_t)a[9] * b[5];
    uint32_t t14 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[6] * b[9] +
            (uint64_t)a[7] * b[8] +
            (uint64_t)a[8] * b[7] +
            (uint64_t)a[9] * b[6];
    uint32_t t15 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[7] * b[9] +
            (uint64_t)a[8] * b[8] +
            (uint64_t)a[9] * b[7];
    uint32_t t16 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[8] * b[9] +
            (uint64_t)a[9] * b[8];
    uint32_t t17 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[9] * b[9];
    uint32_t t18 = c & 0x3FFFFFFUL; c = c >> 26;
    uint32_t t19 = c;

    c = t0 + (uint64_t)t10 * 0x3D10UL;
    t0 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t1 + (uint64_t)t10*0x400UL + (uint64_t)t11 * 0x3D10UL;
    t1 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t2 + (uint64_t)t11*0x400UL + (uint64_t)t12 * 0x3D10UL;
    t2 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t3 + (uint64_t)t12*0x400UL + (uint64_t)t13 * 0x3D10UL;
    r[3] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t4 + (uint64_t)t13*0x400UL + (uint64_t)t14 * 0x3D10UL;
    r[4] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t5 + (uint64_t)t14*0x400UL + (uint64_t)t15 * 0x3D10UL;
    r[5] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t6 + (uint64_t)t15*0x400UL + (uint64_t)t16 * 0x3D10UL;
    r[6] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t7 + (uint64_t)t16*0x400UL + (uint64_t)t17 * 0x3D10UL;
    r[7] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t8 + (uint64_t)t17*0x400UL + (uint64_t)t18 * 0x3D10UL;
    r[8] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t9 + (uint64_t)t18*0x400UL + (uint64_t)t19 * 0x1000003D10ULL;
    r[9] = c & 0x03FFFFFUL; c = c >> 22;
    uint64_t d = t0 + c * 0x3D1UL;
    r[0] = d & 0x3FFFFFFUL; d = d >> 26;
    d = d + t1 + c*0x40;
    r[1] = d & 0x3FFFFFFUL; d = d >> 26;
    r[2] = t2 + d;
}

void inline secp256k1_fe_sqr_inner(const uint32_t *a, uint32_t *r) {
    uint64_t c = (uint64_t)a[0] * a[0];
    uint32_t t0 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[1];
    uint32_t t1 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[2] +
            (uint64_t)a[1] * a[1];
    uint32_t t2 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[3] +
            (uint64_t)(a[1]*2) * a[2];
    uint32_t t3 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[4] +
            (uint64_t)(a[1]*2) * a[3] +
            (uint64_t)a[2] * a[2];
    uint32_t t4 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[5] +
            (uint64_t)(a[1]*2) * a[4] +
            (uint64_t)(a[2]*2) * a[3];
    uint32_t t5 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[6] +
            (uint64_t)(a[1]*2) * a[5] +
            (uint64_t)(a[2]*2) * a[4] +
            (uint64_t)a[3] * a[3];
    uint32_t t6 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[7] +
            (uint64_t)(a[1]*2) * a[6] +
            (uint64_t)(a[2]*2) * a[5] +
            (uint64_t)(a[3]*2) * a[4];
    uint32_t t7 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[8] +
            (uint64_t)(a[1]*2) * a[7] +
            (uint64_t)(a[2]*2) * a[6] +
            (uint64_t)(a[3]*2) * a[5] +
            (uint64_t)a[4] * a[4];
    uint32_t t8 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[0]*2) * a[9] +
            (uint64_t)(a[1]*2) * a[8] +
            (uint64_t)(a[2]*2) * a[7] +
            (uint64_t)(a[3]*2) * a[6] +
            (uint64_t)(a[4]*2) * a[5];
    uint32_t t9 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[1]*2) * a[9] +
            (uint64_t)(a[2]*2) * a[8] +
            (uint64_t)(a[3]*2) * a[7] +
            (uint64_t)(a[4]*2) * a[6] +
            (uint64_t)a[5] * a[5];
    uint32_t t10 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[2]*2) * a[9] +
            (uint64_t)(a[3]*2) * a[8] +
            (uint64_t)(a[4]*2) * a[7] +
            (uint64_t)(a[5]*2) * a[6];
    uint32_t t11 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[3]*2) * a[9] +
            (uint64_t)(a[4]*2) * a[8] +
            (uint64_t)(a[5]*2) * a[7] +
            (uint64_t)a[6] * a[6];
    uint32_t t12 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[4]*2) * a[9] +
            (uint64_t)(a[5]*2) * a[8] +
            (uint64_t)(a[6]*2) * a[7];
    uint32_t t13 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[5]*2) * a[9] +
            (uint64_t)(a[6]*2) * a[8] +
            (uint64_t)a[7] * a[7];
    uint32_t t14 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[6]*2) * a[9] +
            (uint64_t)(a[7]*2) * a[8];
    uint32_t t15 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[7]*2) * a[9] +
            (uint64_t)a[8] * a[8];
    uint32_t t16 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)(a[8]*2) * a[9];
    uint32_t t17 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + (uint64_t)a[9] * a[9];
    uint32_t t18 = c & 0x3FFFFFFUL; c = c >> 26;
    uint32_t t19 = c;

    c = t0 + (uint64_t)t10 * 0x3D10UL;
    t0 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t1 + (uint64_t)t10*0x400UL + (uint64_t)t11 * 0x3D10UL;
    t1 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t2 + (uint64_t)t11*0x400UL + (uint64_t)t12 * 0x3D10UL;
    t2 = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t3 + (uint64_t)t12*0x400UL + (uint64_t)t13 * 0x3D10UL;
    r[3] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t4 + (uint64_t)t13*0x400UL + (uint64_t)t14 * 0x3D10UL;
    r[4] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t5 + (uint64_t)t14*0x400UL + (uint64_t)t15 * 0x3D10UL;
    r[5] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t6 + (uint64_t)t15*0x400UL + (uint64_t)t16 * 0x3D10UL;
    r[6] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t7 + (uint64_t)t16*0x400UL + (uint64_t)t17 * 0x3D10UL;
    r[7] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t8 + (uint64_t)t17*0x400UL + (uint64_t)t18 * 0x3D10UL;
    r[8] = c & 0x3FFFFFFUL; c = c >> 26;
    c = c + t9 + (uint64_t)t18*0x400UL + (uint64_t)t19 * 0x1000003D10ULL;
    r[9] = c & 0x03FFFFFUL; c = c >> 22;
    uint64_t d = t0 + c * 0x3D1UL;
    r[0] = d & 0x3FFFFFFUL; d = d >> 26;
    d = d + t1 + c*0x40;
    r[1] = d & 0x3FFFFFFUL; d = d >> 26;
    r[2] = t2 + d;
}


void secp256k1_fe_mul(secp256k1_fe_t *r, const secp256k1_fe_t *a, const secp256k1_fe_t *b) {
#ifdef VERIFY
    assert(a->magnitude <= 8);
    assert(b->magnitude <= 8);
    r->magnitude = 1;
    r->normalized = 0;
#endif
    secp256k1_fe_mul_inner(a->n, b->n, r->n);
}

void secp256k1_fe_sqr(secp256k1_fe_t *r, const secp256k1_fe_t *a) {
#ifdef VERIFY
    assert(a->magnitude <= 8);
    r->magnitude = 1;
    r->normalized = 0;
#endif
    secp256k1_fe_sqr_inner(a->n, r->n);
}

void secp256k1_fe_get_hex(char *r, int *rlen, const secp256k1_fe_t *a) {
    if (*rlen < 65) {
        *rlen = 65;
        return;
    }
    *rlen = 65;
    unsigned char tmp[32];
    secp256k1_fe_t b = *a;
    secp256k1_fe_normalize(&b);
    secp256k1_fe_get_b32(tmp, &b);
    for (int i=0; i<32; i++) {
        const char *c = "0123456789abcdef";
        r[2*i]   = c[(tmp[i] >> 4) & 0xF];
        r[2*i+1] = c[(tmp[i]) & 0xF];
    }
    r[64] = 0x00;
}

void secp256k1_fe_set_hex(secp256k1_fe_t *r, const char *a, int alen) {
    unsigned char tmp[32] = {};
    const int cvt[256] = {0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 1, 2, 3, 4, 5, 6,7,8,9,0,0,0,0,0,0,
                                 0,10,11,12,13,14,15,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0,10,11,12,13,14,15,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0,
                                 0, 0, 0, 0, 0, 0, 0,0,0,0,0,0,0,0,0,0};
    for (int i=0; i<32; i++) {
        if (alen > i*2)
            tmp[32 - alen/2 + i] = (cvt[(unsigned char)a[2*i]] << 4) + cvt[(unsigned char)a[2*i+1]];
    }
    secp256k1_fe_set_b32(r, tmp);
}

void secp256k1_fe_sqrt(secp256k1_fe_t *r, const secp256k1_fe_t *a) {
    // calculate a^p, with p={15,780,1022,1023}
    secp256k1_fe_t a2; secp256k1_fe_sqr(&a2, a);
    secp256k1_fe_t a3; secp256k1_fe_mul(&a3, &a2, a);
    secp256k1_fe_t a6; secp256k1_fe_sqr(&a6, &a3);
    secp256k1_fe_t a12; secp256k1_fe_sqr(&a12, &a6);
    secp256k1_fe_t a15; secp256k1_fe_mul(&a15, &a12, &a3);
    secp256k1_fe_t a30; secp256k1_fe_sqr(&a30, &a15);
    secp256k1_fe_t a60; secp256k1_fe_sqr(&a60, &a30);
    secp256k1_fe_t a120; secp256k1_fe_sqr(&a120, &a60);
    secp256k1_fe_t a240; secp256k1_fe_sqr(&a240, &a120);
    secp256k1_fe_t a255; secp256k1_fe_mul(&a255, &a240, &a15);
    secp256k1_fe_t a510; secp256k1_fe_sqr(&a510, &a255);
    secp256k1_fe_t a750; secp256k1_fe_mul(&a750, &a510, &a240);
    secp256k1_fe_t a780; secp256k1_fe_mul(&a780, &a750, &a30);
    secp256k1_fe_t a1020; secp256k1_fe_sqr(&a1020, &a510);
    secp256k1_fe_t a1022; secp256k1_fe_mul(&a1022, &a1020, &a2);
    secp256k1_fe_t a1023; secp256k1_fe_mul(&a1023, &a1022, a);
    secp256k1_fe_t x = a15;
    for (int i=0; i<21; i++) {
        for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
        secp256k1_fe_mul(&x, &x, &a1023);
    }
    for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
    secp256k1_fe_mul(&x, &x, &a1022);
    for (int i=0; i<2; i++) {
        for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
        secp256k1_fe_mul(&x, &x, &a1023);
    }
    for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
    secp256k1_fe_mul(r, &x, &a780);
}

void secp256k1_fe_inv(secp256k1_fe_t *r, const secp256k1_fe_t *a) {
    // calculate a^p, with p={45,63,1019,1023}
    secp256k1_fe_t a2; secp256k1_fe_sqr(&a2, a);
    secp256k1_fe_t a3; secp256k1_fe_mul(&a3, &a2, a);
    secp256k1_fe_t a4; secp256k1_fe_sqr(&a4, &a2);
    secp256k1_fe_t a5; secp256k1_fe_mul(&a5, &a4, a);
    secp256k1_fe_t a10; secp256k1_fe_sqr(&a10, &a5);
    secp256k1_fe_t a11; secp256k1_fe_mul(&a11, &a10, a);
    secp256k1_fe_t a21; secp256k1_fe_mul(&a21, &a11, &a10);
    secp256k1_fe_t a42; secp256k1_fe_sqr(&a42, &a21);
    secp256k1_fe_t a45; secp256k1_fe_mul(&a45, &a42, &a3);
    secp256k1_fe_t a63; secp256k1_fe_mul(&a63, &a42, &a21);
    secp256k1_fe_t a126; secp256k1_fe_sqr(&a126, &a63);
    secp256k1_fe_t a252; secp256k1_fe_sqr(&a252, &a126);
    secp256k1_fe_t a504; secp256k1_fe_sqr(&a504, &a252);
    secp256k1_fe_t a1008; secp256k1_fe_sqr(&a1008, &a504);
    secp256k1_fe_t a1019; secp256k1_fe_mul(&a1019, &a1008, &a11);
    secp256k1_fe_t a1023; secp256k1_fe_mul(&a1023, &a1019, &a4);
    secp256k1_fe_t x = a63;
    for (int i=0; i<21; i++) {
        for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
        secp256k1_fe_mul(&x, &x, &a1023);
    }
    for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
    secp256k1_fe_mul(&x, &x, &a1019);
    for (int i=0; i<2; i++) {
        for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
        secp256k1_fe_mul(&x, &x, &a1023);
    }
    for (int j=0; j<10; j++) secp256k1_fe_sqr(&x, &x);
    secp256k1_fe_mul(r, &x, &a45);
}

void secp256k1_fe_inv_var(secp256k1_fe_t *r, const secp256k1_fe_t *a) {
    unsigned char b[32];
    secp256k1_fe_t c = *a;
    secp256k1_fe_normalize(&c);
    secp256k1_fe_get_b32(b, &c);
    secp256k1_num_t n;
    secp256k1_num_init(&n);
    secp256k1_num_set_bin(&n, b, 32);
    secp256k1_num_mod_inverse(&n, &n, &secp256k1_fe_consts->p);
    secp256k1_num_get_bin(b, 32, &n);
    secp256k1_num_free(&n);
    secp256k1_fe_set_b32(r, b);
}

void secp256k1_fe_start(void) {
    const unsigned char secp256k1_fe_consts_p[] = {
        0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,
        0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,
        0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,0xFF,
        0xFF,0xFF,0xFF,0xFE,0xFF,0xFF,0xFC,0x2F
    };
    if (secp256k1_fe_consts == NULL) {
        secp256k1_fe_inner_start();
        secp256k1_fe_consts_t *ret = (secp256k1_fe_consts_t*)malloc(sizeof(secp256k1_fe_consts_t));
        secp256k1_num_init(&ret->p);
        secp256k1_num_set_bin(&ret->p, secp256k1_fe_consts_p, sizeof(secp256k1_fe_consts_p));
        secp256k1_fe_consts = ret;
    }
}

void secp256k1_fe_stop(void) {
    if (secp256k1_fe_consts != NULL) {
        secp256k1_fe_consts_t *c = (secp256k1_fe_consts_t*)secp256k1_fe_consts;
        secp256k1_num_free(&c->p);
        free((void*)c);
        secp256k1_fe_consts = NULL;
        secp256k1_fe_inner_stop();
    }
}
