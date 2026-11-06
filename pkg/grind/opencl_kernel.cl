// Layer searches on an OpenCL device (D208). A layer hash is
// H(entry || nonce): 40 bytes, one padded block for SHA-256 and for
// SHA-512/256. Each work item tries per nonces in a row from base +
// gid*per, keeps the strongest (the lowest nonce among equals) as a key
// (strength+1) << 32 | (0xffffffff - offset), and each work group writes
// the largest key of its items to out[group]. 0 means no nonce tried.

__constant uint K256[64] = {
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
};

#define ROTR32(x, n) rotate((uint)(x), (uint)(32 - (n)))

__kernel void sha256_layers(__global const uint *entry, ulong base, uint n, uint per,
                            __global ulong *out, __local ulong *best) {
	uint gid = get_global_id(0), lid = get_local_id(0), lsz = get_local_size(0);
	uint m[8];
	for (int i = 0; i < 8; i++) m[i] = entry[i];
	ulong mine = 0;
	uint start = gid * per;
	for (uint i = 0; i < per; i++) {
		uint off = start + i;
		if (off >= n) break;
		ulong nonce = base + off;
		uint w[16];
		for (int j = 0; j < 8; j++) w[j] = m[j];
		w[8] = (uint)(nonce >> 32);
		w[9] = (uint)nonce;
		w[10] = 0x80000000u;
		w[11] = 0; w[12] = 0; w[13] = 0; w[14] = 0;
		w[15] = 320;
		uint a = 0x6a09e667u, b = 0xbb67ae85u, c = 0x3c6ef372u, d = 0xa54ff53au;
		uint e = 0x510e527fu, f = 0x9b05688cu, g = 0x1f83d9abu, h = 0x5be0cd19u;
		#pragma unroll
		for (int t = 0; t < 64; t++) {
			uint wt;
			if (t < 16) {
				wt = w[t];
			} else {
				uint x = w[(t - 15) & 15], y = w[(t - 2) & 15];
				uint s0 = ROTR32(x, 7) ^ ROTR32(x, 18) ^ (x >> 3);
				uint s1 = ROTR32(y, 17) ^ ROTR32(y, 19) ^ (y >> 10);
				wt = w[t & 15] + s0 + w[(t - 7) & 15] + s1;
				w[t & 15] = wt;
			}
			uint t1 = h + (ROTR32(e, 6) ^ ROTR32(e, 11) ^ ROTR32(e, 25)) + ((e & f) ^ (~e & g)) + K256[t] + wt;
			uint t2 = (ROTR32(a, 2) ^ ROTR32(a, 13) ^ ROTR32(a, 22)) + ((a & b) ^ (a & c) ^ (b & c));
			h = g; g = f; f = e; e = d + t1; d = c; c = b; b = a; a = t1 + t2;
		}
		uint hv[8] = {a + 0x6a09e667u, b + 0xbb67ae85u, c + 0x3c6ef372u, d + 0xa54ff53au,
		              e + 0x510e527fu, f + 0x9b05688cu, g + 0x1f83d9abu, h + 0x5be0cd19u};
		uint s = 256;
		for (int j = 0; j < 8; j++) {
			if (hv[j] != 0) { s = 32 * j + clz(hv[j]); break; }
		}
		ulong key = ((ulong)(s + 1) << 32) | (ulong)(0xffffffffu - off);
		if (key > mine) mine = key;
	}
	best[lid] = mine;
	barrier(CLK_LOCAL_MEM_FENCE);
	for (uint k = lsz / 2; k > 0; k >>= 1) {
		if (lid < k && best[lid + k] > best[lid]) best[lid] = best[lid + k];
		barrier(CLK_LOCAL_MEM_FENCE);
	}
	if (lid == 0) out[get_group_id(0)] = best[0];
}

__constant ulong K512[80] = {
	0x428a2f98d728ae22UL, 0x7137449123ef65cdUL, 0xb5c0fbcfec4d3b2fUL, 0xe9b5dba58189dbbcUL,
	0x3956c25bf348b538UL, 0x59f111f1b605d019UL, 0x923f82a4af194f9bUL, 0xab1c5ed5da6d8118UL,
	0xd807aa98a3030242UL, 0x12835b0145706fbeUL, 0x243185be4ee4b28cUL, 0x550c7dc3d5ffb4e2UL,
	0x72be5d74f27b896fUL, 0x80deb1fe3b1696b1UL, 0x9bdc06a725c71235UL, 0xc19bf174cf692694UL,
	0xe49b69c19ef14ad2UL, 0xefbe4786384f25e3UL, 0x0fc19dc68b8cd5b5UL, 0x240ca1cc77ac9c65UL,
	0x2de92c6f592b0275UL, 0x4a7484aa6ea6e483UL, 0x5cb0a9dcbd41fbd4UL, 0x76f988da831153b5UL,
	0x983e5152ee66dfabUL, 0xa831c66d2db43210UL, 0xb00327c898fb213fUL, 0xbf597fc7beef0ee4UL,
	0xc6e00bf33da88fc2UL, 0xd5a79147930aa725UL, 0x06ca6351e003826fUL, 0x142929670a0e6e70UL,
	0x27b70a8546d22ffcUL, 0x2e1b21385c26c926UL, 0x4d2c6dfc5ac42aedUL, 0x53380d139d95b3dfUL,
	0x650a73548baf63deUL, 0x766a0abb3c77b2a8UL, 0x81c2c92e47edaee6UL, 0x92722c851482353bUL,
	0xa2bfe8a14cf10364UL, 0xa81a664bbc423001UL, 0xc24b8b70d0f89791UL, 0xc76c51a30654be30UL,
	0xd192e819d6ef5218UL, 0xd69906245565a910UL, 0xf40e35855771202aUL, 0x106aa07032bbd1b8UL,
	0x19a4c116b8d2d0c8UL, 0x1e376c085141ab53UL, 0x2748774cdf8eeb99UL, 0x34b0bcb5e19b48a8UL,
	0x391c0cb3c5c95a63UL, 0x4ed8aa4ae3418acbUL, 0x5b9cca4f7763e373UL, 0x682e6ff3d6b2b8a3UL,
	0x748f82ee5defb2fcUL, 0x78a5636f43172f60UL, 0x84c87814a1f0ab72UL, 0x8cc702081a6439ecUL,
	0x90befffa23631e28UL, 0xa4506cebde82bde9UL, 0xbef9a3f7b2c67915UL, 0xc67178f2e372532bUL,
	0xca273eceea26619cUL, 0xd186b8c721c0c207UL, 0xeada7dd6cde0eb1eUL, 0xf57d4f7fee6ed178UL,
	0x06f067aa72176fbaUL, 0x0a637dc5a2c898a6UL, 0x113f9804bef90daeUL, 0x1b710b35131c471bUL,
	0x28db77f523047d84UL, 0x32caab7b40c72493UL, 0x3c9ebe0a15c9bebcUL, 0x431d67c49c100d4cUL,
	0x4cc5d4becb3e42b6UL, 0x597f299cfc657e2aUL, 0x5fcb6fab3ad6faecUL, 0x6c44198c4a475817UL,
};

#define ROTR64(x, n) rotate((ulong)(x), (ulong)(64 - (n)))

__kernel void sha512_256_layers(__global const ulong *entry, ulong base, uint n, uint per,
                                __global ulong *out, __local ulong *best) {
	uint gid = get_global_id(0), lid = get_local_id(0), lsz = get_local_size(0);
	ulong m[4];
	for (int i = 0; i < 4; i++) m[i] = entry[i];
	ulong mine = 0;
	uint start = gid * per;
	for (uint i = 0; i < per; i++) {
		uint off = start + i;
		if (off >= n) break;
		ulong w[16];
		for (int j = 0; j < 4; j++) w[j] = m[j];
		w[4] = base + off;
		w[5] = 0x8000000000000000UL;
		for (int j = 6; j < 15; j++) w[j] = 0;
		w[15] = 320;
		ulong a = 0x22312194fc2bf72cUL, b = 0x9f555fa3c84c64c2UL, c = 0x2393b86b6f53b151UL, d = 0x963877195940eabdUL;
		ulong e = 0x96283ee2a88effe3UL, f = 0xbe5e1e2553863992UL, g = 0x2b0199fc2c85b8aaUL, h = 0x0eb72ddc81c52ca2UL;
		#pragma unroll
		for (int t = 0; t < 80; t++) {
			ulong wt;
			if (t < 16) {
				wt = w[t];
			} else {
				ulong x = w[(t - 15) & 15], y = w[(t - 2) & 15];
				ulong s0 = ROTR64(x, 1) ^ ROTR64(x, 8) ^ (x >> 7);
				ulong s1 = ROTR64(y, 19) ^ ROTR64(y, 61) ^ (y >> 6);
				wt = w[t & 15] + s0 + w[(t - 7) & 15] + s1;
				w[t & 15] = wt;
			}
			ulong t1 = h + (ROTR64(e, 14) ^ ROTR64(e, 18) ^ ROTR64(e, 41)) + ((e & f) ^ (~e & g)) + K512[t] + wt;
			ulong t2 = (ROTR64(a, 28) ^ ROTR64(a, 34) ^ ROTR64(a, 39)) + ((a & b) ^ (a & c) ^ (b & c));
			h = g; g = f; f = e; e = d + t1; d = c; c = b; b = a; a = t1 + t2;
		}
		ulong hv[4] = {a + 0x22312194fc2bf72cUL, b + 0x9f555fa3c84c64c2UL, c + 0x2393b86b6f53b151UL, d + 0x963877195940eabdUL};
		uint s = 256;
		for (int j = 0; j < 4; j++) {
			if (hv[j] != 0) { s = 64 * j + (uint)clz(hv[j]); break; }
		}
		ulong key = ((ulong)(s + 1) << 32) | (ulong)(0xffffffffu - off);
		if (key > mine) mine = key;
	}
	best[lid] = mine;
	barrier(CLK_LOCAL_MEM_FENCE);
	for (uint k = lsz / 2; k > 0; k >>= 1) {
		if (lid < k && best[lid + k] > best[lid]) best[lid] = best[lid + k];
		barrier(CLK_LOCAL_MEM_FENCE);
	}
	if (lid == 0) out[get_group_id(0)] = best[0];
}
