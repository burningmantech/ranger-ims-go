//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

//! A port of golang.org/x/crypto/argon2's Argon2id, used only for hashes whose
//! memory cost is below Argon2's minimum of 8 KiB per lane. Go accepts those:
//! it hashes the given cost into H0 but then quietly allocates the minimum
//! number of blocks, which no RFC 9106 implementation (including the argon2
//! crate) reproduces. The API integration suite's seed users have such hashes.

use blake2::digest::{Update, VariableOutput};
use blake2::{Blake2b512, Blake2bVar, Digest};

const BLOCK_LENGTH: usize = 128;
const SYNC_POINTS: u32 = 4;
const ARGON2ID: u32 = 2;
const VERSION: u32 = 0x13;

type Block = [u64; BLOCK_LENGTH];

pub fn id_key(
    password: &[u8],
    salt: &[u8],
    time: u32,
    memory: u32,
    threads: u8,
    key_len: u32,
) -> Vec<u8> {
    assert!(time >= 1, "argon2: number of rounds too small");
    assert!(threads >= 1, "argon2: parallelism degree too low");
    let threads = u32::from(threads);
    let mut h0 = init_hash(password, salt, time, memory, threads, key_len);

    let mut memory = memory / (SYNC_POINTS * threads) * (SYNC_POINTS * threads);
    if memory < 2 * SYNC_POINTS * threads {
        memory = 2 * SYNC_POINTS * threads;
    }
    let mut b = init_blocks(&mut h0, memory, threads);
    process_blocks(&mut b, time, memory, threads);
    extract_key(&mut b, memory, threads, key_len)
}

fn init_hash(
    password: &[u8],
    salt: &[u8],
    time: u32,
    memory: u32,
    threads: u32,
    key_len: u32,
) -> [u8; 72] {
    let mut b2 = Blake2b512::new();
    for v in [threads, key_len, memory, time, VERSION, ARGON2ID] {
        Digest::update(&mut b2, v.to_le_bytes());
    }
    for data in [password, salt, &[], &[]] {
        Digest::update(&mut b2, (data.len() as u32).to_le_bytes());
        Digest::update(&mut b2, data);
    }
    let mut h0 = [0u8; 72];
    h0[..64].copy_from_slice(&b2.finalize());
    h0
}

/// Argon2's variable-length hash H'.
fn blake2b_hash(out: &mut [u8], input: &[u8]) {
    let out_len = out.len();
    let len_prefix = (out_len as u32).to_le_bytes();
    if out_len <= 64 {
        let mut b2 = Blake2bVar::new(out_len).expect("valid blake2b output length");
        b2.update(&len_prefix);
        b2.update(input);
        b2.finalize_variable(out).expect("matching output length");
        return;
    }
    let mut b2 = Blake2b512::new();
    Digest::update(&mut b2, len_prefix);
    Digest::update(&mut b2, input);
    let mut buffer: [u8; 64] = b2.finalize().into();
    out[..32].copy_from_slice(&buffer[..32]);
    let mut pos = 32;
    while out_len - pos > 64 {
        buffer = Blake2b512::digest(buffer).into();
        out[pos..pos + 32].copy_from_slice(&buffer[..32]);
        pos += 32;
    }
    let last_len = out_len - pos;
    let mut b2 = Blake2bVar::new(last_len).expect("valid blake2b output length");
    b2.update(&buffer);
    b2.finalize_variable(&mut out[pos..])
        .expect("matching output length");
}

fn init_blocks(h0: &mut [u8; 72], memory: u32, threads: u32) -> Vec<Block> {
    let mut b = vec![[0u64; BLOCK_LENGTH]; memory as usize];
    let mut block0 = [0u8; 1024];
    for lane in 0..threads {
        let j = (lane * (memory / threads)) as usize;
        h0[68..72].copy_from_slice(&lane.to_le_bytes());
        for k in 0..2u32 {
            h0[64..68].copy_from_slice(&k.to_le_bytes());
            blake2b_hash(&mut block0, h0);
            for (i, word) in b[j + k as usize].iter_mut().enumerate() {
                *word = u64::from_le_bytes(block0[i * 8..i * 8 + 8].try_into().unwrap());
            }
        }
    }
    b
}

fn process_blocks(b: &mut [Block], time: u32, memory: u32, threads: u32) {
    let lanes = memory / threads;
    let segments = lanes / SYNC_POINTS;
    for n in 0..time {
        for slice in 0..SYNC_POINTS {
            // Go runs the lanes of a slice concurrently; they touch disjoint
            // blocks for writing, so running them in turn gives the same result.
            for lane in 0..threads {
                process_segment(b, n, slice, lane, lanes, segments, memory, time, threads);
            }
        }
    }
}

#[allow(clippy::too_many_arguments)]
fn process_segment(
    b: &mut [Block],
    n: u32,
    slice: u32,
    lane: u32,
    lanes: u32,
    segments: u32,
    memory: u32,
    time: u32,
    threads: u32,
) {
    let mut addresses: Block = [0; BLOCK_LENGTH];
    let mut input: Block = [0; BLOCK_LENGTH];
    let zero: Block = [0; BLOCK_LENGTH];
    let data_independent = n == 0 && slice < SYNC_POINTS / 2;
    if data_independent {
        input[0] = u64::from(n);
        input[1] = u64::from(lane);
        input[2] = u64::from(slice);
        input[3] = u64::from(memory);
        input[4] = u64::from(time);
        input[5] = u64::from(ARGON2ID);
    }

    let mut index = 0u32;
    if n == 0 && slice == 0 {
        index = 2;
        input[6] += 1;
        next_addresses(&mut addresses, &input, &zero);
    }

    let mut offset = lane * lanes + slice * segments + index;
    while index < segments {
        let mut prev = offset.wrapping_sub(1);
        if index == 0 && slice == 0 {
            prev = prev.wrapping_add(lanes);
        }
        let random = if data_independent {
            if (index as usize).is_multiple_of(BLOCK_LENGTH) {
                input[6] += 1;
                next_addresses(&mut addresses, &input, &zero);
            }
            addresses[index as usize % BLOCK_LENGTH]
        } else {
            b[prev as usize][0]
        };
        let new_offset = index_alpha(random, lanes, segments, threads, n, slice, lane, index);
        let prev_block = b[prev as usize];
        let ref_block = b[new_offset as usize];
        process_block(&mut b[offset as usize], &prev_block, &ref_block, true);
        index += 1;
        offset += 1;
    }
}

fn next_addresses(addresses: &mut Block, input: &Block, zero: &Block) {
    process_block(addresses, input, zero, false);
    let first = *addresses;
    process_block(addresses, &first, zero, false);
}

#[allow(clippy::too_many_arguments)]
fn index_alpha(
    rand: u64,
    lanes: u32,
    segments: u32,
    threads: u32,
    n: u32,
    slice: u32,
    lane: u32,
    index: u32,
) -> u32 {
    let mut ref_lane = ((rand >> 32) as u32) % threads;
    if n == 0 && slice == 0 {
        ref_lane = lane;
    }
    let mut m = 3 * segments;
    let mut s = ((slice + 1) % SYNC_POINTS) * segments;
    if lane == ref_lane {
        m += index;
    }
    if n == 0 {
        m = slice * segments;
        s = 0;
        if slice == 0 || lane == ref_lane {
            m += index;
        }
    }
    if index == 0 || lane == ref_lane {
        m -= 1;
    }
    phi(rand, u64::from(m), u64::from(s), ref_lane, lanes)
}

fn phi(rand: u64, m: u64, s: u64, lane: u32, lanes: u32) -> u32 {
    let mut p = rand & 0xFFFF_FFFF;
    p = (p * p) >> 32;
    p = (p * m) >> 32;
    lane * lanes + ((s + m - (p + 1)) % u64::from(lanes)) as u32
}

fn process_block(out: &mut Block, in1: &Block, in2: &Block, xor: bool) {
    let mut t: Block = [0; BLOCK_LENGTH];
    for i in 0..BLOCK_LENGTH {
        t[i] = in1[i] ^ in2[i];
    }
    for i in (0..BLOCK_LENGTH).step_by(16) {
        blamka(
            &mut t,
            [
                i,
                i + 1,
                i + 2,
                i + 3,
                i + 4,
                i + 5,
                i + 6,
                i + 7,
                i + 8,
                i + 9,
                i + 10,
                i + 11,
                i + 12,
                i + 13,
                i + 14,
                i + 15,
            ],
        );
    }
    for i in (0..BLOCK_LENGTH / 8).step_by(2) {
        blamka(
            &mut t,
            [
                i,
                i + 1,
                16 + i,
                16 + i + 1,
                32 + i,
                32 + i + 1,
                48 + i,
                48 + i + 1,
                64 + i,
                64 + i + 1,
                80 + i,
                80 + i + 1,
                96 + i,
                96 + i + 1,
                112 + i,
                112 + i + 1,
            ],
        );
    }
    for i in 0..BLOCK_LENGTH {
        let v = in1[i] ^ in2[i] ^ t[i];
        if xor {
            out[i] ^= v;
        } else {
            out[i] = v;
        }
    }
}

fn gb(v: &mut [u64; 16], a: usize, b: usize, c: usize, d: usize) {
    let fbl = |x: u64, y: u64| {
        x.wrapping_add(y).wrapping_add(
            2u64.wrapping_mul(u64::from(x as u32))
                .wrapping_mul(u64::from(y as u32)),
        )
    };
    v[a] = fbl(v[a], v[b]);
    v[d] = (v[d] ^ v[a]).rotate_right(32);
    v[c] = fbl(v[c], v[d]);
    v[b] = (v[b] ^ v[c]).rotate_right(24);
    v[a] = fbl(v[a], v[b]);
    v[d] = (v[d] ^ v[a]).rotate_right(16);
    v[c] = fbl(v[c], v[d]);
    v[b] = (v[b] ^ v[c]).rotate_right(63);
}

fn blamka(t: &mut Block, idx: [usize; 16]) {
    let mut v = [0u64; 16];
    for (k, &i) in idx.iter().enumerate() {
        v[k] = t[i];
    }
    gb(&mut v, 0, 4, 8, 12);
    gb(&mut v, 1, 5, 9, 13);
    gb(&mut v, 2, 6, 10, 14);
    gb(&mut v, 3, 7, 11, 15);
    gb(&mut v, 0, 5, 10, 15);
    gb(&mut v, 1, 6, 11, 12);
    gb(&mut v, 2, 7, 8, 13);
    gb(&mut v, 3, 4, 9, 14);
    for (k, &i) in idx.iter().enumerate() {
        t[i] = v[k];
    }
}

fn extract_key(b: &mut [Block], memory: u32, threads: u32, key_len: u32) -> Vec<u8> {
    let lanes = memory / threads;
    let last = (memory - 1) as usize;
    for lane in 0..threads - 1 {
        let src = b[((lane * lanes) + lanes - 1) as usize];
        for (i, v) in src.iter().enumerate() {
            b[last][i] ^= v;
        }
    }
    let mut block = [0u8; 1024];
    for (i, v) in b[last].iter().enumerate() {
        block[i * 8..i * 8 + 8].copy_from_slice(&v.to_le_bytes());
    }
    let mut key = vec![0u8; key_len as usize];
    blake2b_hash(&mut key, &block);
    key
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matches_the_argon2_crate_for_conforming_params() {
        // Where Go follows the RFC, this port must agree with the crate.
        let params = argon2::Params::new(64, 2, 2, Some(32)).unwrap();
        let mut expected = [0u8; 32];
        argon2::Argon2::new(argon2::Algorithm::Argon2id, argon2::Version::V0x13, params)
            .hash_password_into(b"password", b"somesaltsomesalt", &mut expected)
            .unwrap();
        assert_eq!(
            id_key(b"password", b"somesaltsomesalt", 2, 64, 2, 32),
            expected
        );
        // And golang.org/x/crypto/argon2.IDKey with the same inputs gives:
        assert_eq!(
            base16(&expected),
            "c263317d002f84b5342fa66fa169e4bce6a356a13f19193346322ef3522e0702"
        );
    }

    fn base16(b: &[u8]) -> String {
        b.iter().map(|x| format!("{x:02x}")).collect()
    }

    #[test]
    fn matches_the_argon2_crate_for_long_keys() {
        let params = argon2::Params::new(32, 1, 1, Some(100)).unwrap();
        let mut expected = [0u8; 100];
        argon2::Argon2::new(argon2::Algorithm::Argon2id, argon2::Version::V0x13, params)
            .hash_password_into(b"pw", b"saltsaltsalt", &mut expected)
            .unwrap();
        assert_eq!(
            id_key(b"pw", b"saltsaltsalt", 1, 32, 1, 100),
            expected.to_vec()
        );
    }
}
