// Independent test oracle. Build with: cc -O2 f128_mpfr.c -lmpfr -lgmp
// MPFR's subnormalize operation implements IEEE gradual underflow without
// double rounding. No code from this program is linked into the Go runtime.
#include <inttypes.h>
#include <mpfr.h>
#include <stdio.h>
#include <string.h>

static void input(mpfr_t x, uint64_t hi, uint64_t lo) {
    int sign = hi >> 63 ? -1 : 1;
    unsigned e = (hi >> 48) & 32767;
    uint64_t words[2] = {lo, hi & UINT64_C(0xffffffffffff)};
    if (e == 32767) {
        if (words[0] || words[1]) mpfr_set_nan(x);
        else mpfr_set_inf(x, sign);
        return;
    }
    if (!e && !words[0] && !words[1]) { mpfr_set_zero(x, sign); return; }
    if (e) words[1] |= UINT64_C(1) << 48;
    else e = 1;
    mpz_t n;
    mpz_init(n);
    mpz_import(n, 2, -1, sizeof(uint64_t), 0, 0, words);
    mpfr_set_z(x, n, MPFR_RNDN);
    mpfr_mul_2si(x, x, (long)e - 16383 - 112, MPFR_RNDN);
    if (sign < 0) mpfr_neg(x, x, MPFR_RNDN);
    mpz_clear(n);
}

static void output(mpfr_t x, uint64_t *hi, uint64_t *lo) {
    *lo = 0;
    *hi = mpfr_signbit(x) ? UINT64_C(1) << 63 : 0;
    if (mpfr_nan_p(x)) { *hi = UINT64_C(0x7fff800000000000); return; }
    if (mpfr_inf_p(x)) { *hi |= UINT64_C(0x7fff000000000000); return; }
    if (mpfr_zero_p(x)) return;
    mpz_t n;
    mpz_init(n);
    long scale = mpfr_get_z_2exp(n, x);
    mpz_abs(n, n);
    long e = mpfr_get_exp(x) - 1;
    if (e < -16382) e = -16382;
    long shift = scale - (e - 112);
    if (shift < 0) mpz_fdiv_q_2exp(n, n, -shift);
    else mpz_mul_2exp(n, n, shift);
    if (mpz_tstbit(n, 112)) *hi |= (uint64_t)(e + 16383) << 48;
    mpz_clrbit(n, 112);
    uint64_t words[2] = {0, 0};
    size_t count;
    mpz_export(words, &count, -1, sizeof(uint64_t), 0, 0, n);
    *hi |= words[1];
    *lo = words[0];
    mpz_clear(n);
}

int main(void) {
    mpfr_set_emin(-16493);
    mpfr_set_emax(16384);
    mpfr_t x, y, r;
    mpfr_inits2(113, x, y, r, (mpfr_ptr)0);
    char op[32];
    uint64_t xh, xl, yh, yl, rh, rl;
    while (scanf("%31s %"SCNx64" %"SCNx64" %"SCNx64" %"SCNx64, op, &xh, &xl, &yh, &yl) == 5) {
        input(x, xh, xl);
        input(y, yh, yl);
        int inexact = 0, sign = 0;
#define UNARY(name) if (!strcmp(op, #name)) inexact = mpfr_##name(r, x, MPFR_RNDN); else
        UNARY(exp) UNARY(exp2) UNARY(expm1) UNARY(log) UNARY(log2) UNARY(log10) UNARY(log1p)
        UNARY(sin) UNARY(cos) UNARY(tan) UNARY(asin) UNARY(acos) UNARY(atan)
        UNARY(sinh) UNARY(cosh) UNARY(tanh) UNARY(asinh) UNARY(acosh) UNARY(atanh)
        UNARY(cbrt) UNARY(erf) UNARY(erfc) UNARY(gamma)
        if (!strcmp(op, "lgamma")) inexact = mpfr_lgamma(r, &sign, x, MPFR_RNDN);
        else if (!strcmp(op, "pow")) inexact = mpfr_pow(r, x, y, MPFR_RNDN);
        else if (!strcmp(op, "atan2")) inexact = mpfr_atan2(r, x, y, MPFR_RNDN);
        else if (!strcmp(op, "hypot")) inexact = mpfr_hypot(r, x, y, MPFR_RNDN);
        else { fprintf(stderr, "unknown operation: %s\n", op); return 1; }
        mpfr_subnormalize(r, inexact, MPFR_RNDN);
        output(r, &rh, &rl);
        printf("%s %016"PRIx64" %016"PRIx64" %016"PRIx64" %016"PRIx64" %016"PRIx64" %016"PRIx64" %d\n", op, xh, xl, yh, yl, rh, rl, sign);
    }
    mpfr_clears(x, y, r, (mpfr_ptr)0);
    return ferror(stdin) ? 1 : 0;
}
