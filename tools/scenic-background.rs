//! Original voxel river valley for LXMCL; no game textures or reference pixels.
//! Renderer: Rust standard library only. Deterministic scene, textures and lighting.
//! rustc -O tools/scenic-background.rs -o target/scenic-background.exe
//! target/scenic-background.exe target/scenic-home.ppm
//! Convert the PPM with FFmpeg: ffmpeg -i target/scenic-home.ppm ui/assets/scenic-home.png
//! Alternatively enable the `png` Cargo feature with image 0.25 (png feature).
//! Usage: scenic-background [output.ppm] [output.png] [width] [height]
//! Generated artwork and this generator: original LXMCL contribution, CC0-1.0.

use std::fs::File;
use std::io::{BufWriter, Write};
use std::ops::{Add, Div, Mul, Sub};
use std::sync::Arc;
use std::thread;

const NX: i32 = 384;
const NY: i32 = 150;
const NZ: i32 = 512;
const WATER: f64 = 13.0;
const AIR: u8 = 0;
const GRASS: u8 = 1;
const DIRT: u8 = 2;
const STONE: u8 = 3;
const SAND: u8 = 4;
const WOOD: u8 = 5;
const LEAF: u8 = 6;
const BIRCH: u8 = 7;
const CLOUD: u8 = 8;
const SNOW: u8 = 9;
const FLOWER: u8 = 10;
const HERB: u8 = 11;

#[derive(Clone, Copy, Default, Debug)]
struct V(f64, f64, f64);
impl Add for V {
    type Output = V;
    fn add(self, v: V) -> V {
        V(self.0 + v.0, self.1 + v.1, self.2 + v.2)
    }
}
impl Sub for V {
    type Output = V;
    fn sub(self, v: V) -> V {
        V(self.0 - v.0, self.1 - v.1, self.2 - v.2)
    }
}
impl Mul<f64> for V {
    type Output = V;
    fn mul(self, k: f64) -> V {
        V(self.0 * k, self.1 * k, self.2 * k)
    }
}
impl Div<f64> for V {
    type Output = V;
    fn div(self, k: f64) -> V {
        self * (1.0 / k)
    }
}
impl V {
    fn dot(self, v: V) -> f64 {
        self.0 * v.0 + self.1 * v.1 + self.2 * v.2
    }
    fn cross(self, v: V) -> V {
        V(
            self.1 * v.2 - self.2 * v.1,
            self.2 * v.0 - self.0 * v.2,
            self.0 * v.1 - self.1 * v.0,
        )
    }
    fn unit(self) -> V {
        self / self.dot(self).sqrt()
    }
    fn lerp(self, v: V, t: f64) -> V {
        self * (1.0 - t) + v * t
    }
    fn rgb(self) -> [u8; 3] {
        [
            (self.0.clamp(0., 255.)) as u8,
            (self.1.clamp(0., 255.)) as u8,
            (self.2.clamp(0., 255.)) as u8,
        ]
    }
}
fn hash(x: i32, y: i32, z: i32) -> f64 {
    let mut h = (x as u32).wrapping_mul(374761393)
        ^ (y as u32).wrapping_mul(668265263)
        ^ (z as u32).wrapping_mul(2147483647);
    h = (h ^ (h >> 13)).wrapping_mul(1274126177);
    h ^= h >> 16;
    h as f64 / u32::MAX as f64
}
fn smooth(t: f64) -> f64 {
    t * t * (3. - 2. * t)
}
fn noise(x: f64, z: f64) -> f64 {
    let ix = x.floor() as i32;
    let iz = z.floor() as i32;
    let u = smooth(x - ix as f64);
    let v = smooth(z - iz as f64);
    let a = hash(ix, 7, iz) * (1. - u) + hash(ix + 1, 7, iz) * u;
    let b = hash(ix, 7, iz + 1) * (1. - u) + hash(ix + 1, 7, iz + 1) * u;
    a * (1. - v) + b * v
}
fn river(z: f64) -> (f64, f64) {
    (
        -10. + 20. * (z * 0.033).sin() + 14. * (z * 0.012).sin(),
        17. + 6. * (z * 0.013 + 1.).sin(),
    )
}
fn elevation(x: f64, z: f64) -> i32 {
    let (center, width) = river(z);
    let bank = (x - center).abs() - width;
    let hills = noise(x * 0.045, z * 0.038) * 6. + noise(x * 0.017 + 5., z * 0.019) * 6.;
    let mut h = if bank < -3. {
        8. + hills * 0.25
    } else {
        12. + (bank.max(0.) * 0.16).min(15.) + hills * (bank.max(0.) / 15.).min(1.)
    };
    // Separated, terraced mountain ridges rise behind the river valley.
    for &(px, pz, height, radius) in &[
        (-105., 266., 48., 83.),
        (-57., 341., 62., 77.),
        (59., 285., 99., 71.),
        (100., 307., 116., 90.),
        (161., 279., 93., 66.),
        (-166., 342., 98., 88.),
        (6., 397., 102., 94.),
    ] {
        let dx = (x - px).abs();
        let dz = (z - pz).abs();
        let d = (dx * dx + dz * dz * 0.65).sqrt();
        let ridge = 0.55 + noise(x * 0.049 + 11., z * 0.049) * 0.64;
        let peak = height * (1. - d / radius).max(0.) * ridge;
        let rugged = 7. * noise(x * 0.10 + 8., z * 0.10) - 3.5;
        if peak > 0. {
            h = h.max(13. + peak + rugged * (peak / 20.).min(1.));
        }
    }
    h.floor() as i32
}
struct World {
    cells: Vec<u8>,
}
#[derive(Clone, Copy)]
struct Hit {
    p: V,
    n: V,
    id: u8,
    t: f64,
}
fn plant_hit(o: V, d: V, x: i32, y: i32, z: i32, id: u8) -> Option<Hit> {
    let origin = V(x as f64, y as f64, z as f64);
    let boxes: &[(V, V)] = if id == FLOWER {
        &[
            (V(0.46, 0., 0.46), V(0.54, 0.62, 0.54)),
            (V(0.31, 0.24, 0.45), V(0.49, 0.31, 0.54)),
            (V(0.31, 0.59, 0.39), V(0.69, 0.68, 0.61)),
            (V(0.39, 0.59, 0.31), V(0.61, 0.68, 0.69)),
            (V(0.44, 0.68, 0.44), V(0.56, 0.73, 0.56)),
        ]
    } else {
        &[
            (V(0.16, 0., 0.31), V(0.23, 0.46, 0.39)),
            (V(0.30, 0., 0.67), V(0.38, 0.66, 0.73)),
            (V(0.49, 0., 0.43), V(0.57, 0.81, 0.50)),
            (V(0.68, 0., 0.19), V(0.76, 0.50, 0.27)),
            (V(0.76, 0., 0.71), V(0.83, 0.61, 0.78)),
        ]
    };
    let mut result: Option<Hit> = None;
    for &(lo, hi) in boxes {
        let lo = origin + lo;
        let hi = origin + hi;
        let mut near: f64 = 0.;
        let mut far = f64::INFINITY;
        let mut normal = V(0., 1., 0.);
        for (a, b, l, h, axis) in [
            (o.0, d.0, lo.0, hi.0, V(1., 0., 0.)),
            (o.1, d.1, lo.1, hi.1, V(0., 1., 0.)),
            (o.2, d.2, lo.2, hi.2, V(0., 0., 1.)),
        ] {
            if b.abs() < 1e-12 {
                if a < l || a > h {
                    far = -1.;
                }
            } else {
                let t0 = (l - a) / b;
                let t1 = (h - a) / b;
                if t0.min(t1) > near {
                    near = t0.min(t1);
                    normal = axis * if b > 0. { -1. } else { 1. };
                }
                far = far.min(t0.max(t1));
            }
        }
        if near < far && near > 0. && near < result.map_or(f64::INFINITY, |h| h.t) {
            result = Some(Hit {
                p: o + d * near,
                n: normal,
                id,
                t: near,
            });
        }
    }
    result
}
impl World {
    fn index(x: i32, y: i32, z: i32) -> usize {
        ((z * NY + y) * NX + x) as usize
    }
    fn get(&self, x: i32, y: i32, z: i32) -> u8 {
        if x < 0 || x >= NX || y < 0 || y >= NY || z < 0 || z >= NZ {
            AIR
        } else {
            self.cells[Self::index(x, y, z)]
        }
    }
    fn set(&mut self, x: i32, y: i32, z: i32, id: u8) {
        if x >= 0 && x < NX && y >= 0 && y < NY && z >= 0 && z < NZ {
            let i = Self::index(x, y, z);
            self.cells[i] = id;
        }
    }
    fn box_fill(&mut self, x: i32, y: i32, z: i32, w: i32, h: i32, d: i32, id: u8) {
        for zz in z..z + d {
            for yy in y..y + h {
                for xx in x..x + w {
                    self.set(xx, yy, zz, id);
                }
            }
        }
    }
    fn ray(&self, o: V, d: V, max: f64) -> Option<Hit> {
        let mut enter: f64 = 0.;
        let mut exit = max;
        for (a, b, limit) in [(o.0, d.0, NX), (o.1, d.1, NY), (o.2, d.2, NZ)] {
            if b.abs() < 1e-12 {
                if a < 0. || a >= limit as f64 {
                    return None;
                }
            } else {
                let t0 = (0. - a) / b;
                let t1 = (limit as f64 - a) / b;
                enter = enter.max(t0.min(t1));
                exit = exit.min(t0.max(t1));
            }
        }
        if enter > exit {
            return None;
        }
        let start = o + d * (enter + 0.00001);
        let mut x = start.0.floor() as i32;
        let mut y = start.1.floor() as i32;
        let mut z = start.2.floor() as i32;
        let sx = if d.0 > 0. { 1 } else { -1 };
        let sy = if d.1 > 0. { 1 } else { -1 };
        let sz = if d.2 > 0. { 1 } else { -1 };
        let dx = (1. / d.0).abs();
        let dy = (1. / d.1).abs();
        let dz = (1. / d.2).abs();
        let mut tx = if sx > 0 {
            (x as f64 + 1. - o.0) / d.0
        } else {
            (x as f64 - o.0) / d.0
        };
        let mut ty = if sy > 0 {
            (y as f64 + 1. - o.1) / d.1
        } else {
            (y as f64 - o.1) / d.1
        };
        let mut tz = if sz > 0 {
            (z as f64 + 1. - o.2) / d.2
        } else {
            (z as f64 - o.2) / d.2
        };
        let mut t = enter;
        let mut n = V(0., 1., 0.);
        loop {
            let id = self.get(x, y, z);
            if id == FLOWER || id == HERB {
                if let Some(h) = plant_hit(o, d, x, y, z, id) {
                    return Some(h);
                }
            } else if id != AIR {
                return Some(Hit {
                    p: o + d * t,
                    n,
                    id,
                    t,
                });
            }
            if tx < ty && tx < tz {
                t = tx;
                tx += dx;
                x += sx;
                n = V(-sx as f64, 0., 0.);
            } else if ty < tz {
                t = ty;
                ty += dy;
                y += sy;
                n = V(0., -sy as f64, 0.);
            } else {
                t = tz;
                tz += dz;
                z += sz;
                n = V(0., 0., -sz as f64);
            }
            if t > exit || x < 0 || x >= NX || y < 0 || y >= NY || z < 0 || z >= NZ {
                return None;
            }
        }
    }
}
fn build() -> World {
    let mut w = World {
        cells: vec![AIR; (NX * NY * NZ) as usize],
    };
    for z in 0..NZ {
        for x in 0..NX {
            let xx = x as f64 - NX as f64 * 0.5;
            let h = elevation(xx, z as f64);
            let (c, r) = river(z as f64);
            let shore = ((xx - c).abs() - r).abs() < 3.0;
            for y in 0..=h {
                let id = if y == h {
                    if h > 76 + (noise(x as f64 * 0.07, z as f64 * 0.07) * 9.) as i32 {
                        SNOW
                    } else if h > 49 {
                        STONE
                    } else if h <= 13 || shore && h < 16 {
                        SAND
                    } else {
                        GRASS
                    }
                } else if h - y < 4 && h < 48 {
                    DIRT
                } else {
                    STONE
                };
                w.set(x, y, z, id);
            }
        }
    }
    // Trees grow in irregular groves on both riverbanks, with deliberate gaps.
    for gz in 0..65 {
        for gx in 0..48 {
            let z = 5 + gz * 8 + (hash(gx, 2, gz) * 8.) as i32;
            let x = 5 + gx * 8 + (hash(gx, 3, gz) * 8.) as i32;
            let xx = x as f64 - 192.;
            let (c, r) = river(z as f64);
            if (xx - c).abs() < r + 10. || hash(gx, 4, gz) < 0.48 {
                continue;
            }
            let h = elevation(xx, z as f64);
            if h < 15 || h > 46 {
                continue;
            }
            let birch = hash(gx, 5, gz) > 0.72;
            let trunk = if birch { BIRCH } else { WOOD };
            let ht = 5 + (hash(gx, 6, gz) * 6.) as i32;
            let radius: i32 = if birch {
                3
            } else {
                3 + (hash(gx, 17, gz) * 2.) as i32
            };
            for y in h + 1..h + ht + 3 {
                w.set(x, y, z, trunk);
            }
            for dy in -2i32..=3 {
                let rr = if dy == 3 {
                    radius - 2
                } else if dy == 2 || dy == -2 {
                    radius - 1
                } else {
                    radius
                };
                for dz in -rr..=rr {
                    for dx in -rr..=rr {
                        if dx.abs() == rr && dz.abs() == rr && hash(x + dx, h + dy, z + dz) < 0.83 {
                            continue;
                        }
                        if hash(x + dx, h + ht + dy, z + dz) < 0.035 {
                            continue;
                        }
                        w.set(x + dx, h + ht + dy, z + dz, LEAF);
                    }
                }
            }
            // A few tiny sunlit block flowers across the meadow, not an asset texture.
        }
    }
    for z in 2..175 {
        for x in 4..NX - 4 {
            let h = elevation(x as f64 - 192., z as f64);
            if w.get(x, h, z) == GRASS && w.get(x, h + 1, z) == AIR {
                let plant = hash(x, 41, z);
                if plant > 0.972 {
                    w.set(x, h + 1, z, FLOWER);
                } else if plant > 0.73 {
                    w.set(x, h + 1, z, HERB);
                }
            }
        }
    }
    // Broad low-profile cuboid clouds, distributed in true perspective.
    for &(x, y, z, ww, dd) in &[
        (88, 84, 74, 19, 7),
        (237, 88, 85, 26, 8),
        (152, 86, 127, 23, 9),
        (304, 92, 132, 36, 11),
        (48, 91, 162, 35, 10),
        (209, 89, 182, 28, 12),
        (113, 92, 232, 39, 13),
        (283, 94, 255, 36, 14),
        (174, 94, 299, 45, 15),
        (345, 96, 337, 43, 16),
        (59, 97, 354, 42, 18),
        (230, 99, 402, 54, 16),
    ] {
        w.box_fill(x, y, z, ww, 2, dd, CLOUD);
        w.box_fill(
            x + 3,
            y + 2,
            z + 2,
            (ww * 3 / 5).max(1),
            1,
            (dd * 2 / 3).max(1),
            CLOUD,
        );
    }
    w
}
fn sky(d: V) -> V {
    let h = d.1.max(0.).powf(0.55);
    let mut c = V(186., 220., 243.).lerp(V(53., 145., 224.), (h * 1.1).min(1.));
    let sun = V(-0.47, 0.78, -0.34).unit();
    let glow = d.dot(sun).max(0.);
    c = c + V(30., 22., 9.) * glow.powf(12.);
    c
}
fn albedo(h: Hit) -> V {
    let p = h.p - h.n * 0.001;
    let bx = p.0.floor() as i32;
    let by = p.1.floor() as i32;
    let bz = p.2.floor() as i32;
    let (u, v) = if h.n.1.abs() > 0.5 {
        (p.0.fract(), p.2.fract())
    } else if h.n.0.abs() > 0.5 {
        (p.2.fract(), p.1.fract())
    } else {
        (p.0.fract(), p.1.fract())
    };
    let tx = (u * 16.).floor() as i32;
    let ty = (v * 16.).floor() as i32;
    let tex = hash(bx * 16 + tx, by * 16 + ty, bz * 13);
    let coarse = hash(bx * 4 + tx / 4, by * 4 + ty / 4, bz);
    let block = hash(bx, by, bz);
    let variation = 0.85 + 0.15 * block + 0.17 * (tex - 0.5) + 0.12 * (coarse - 0.5);
    let c = match h.id {
        GRASS if h.n.1 > 0.5 => V(128., 173., 53.),
        GRASS if v > 0.80 + hash(bx + tx, by, bz) * 0.13 => V(100., 144., 43.),
        GRASS | DIRT => V(132., 94., 61.),
        SAND => V(199., 184., 132.),
        STONE => {
            let seam = if ty % 5 == 0 && tex > 0.63 { 0.83 } else { 1. };
            V(141., 150., 153.) * seam
        }
        SNOW => V(234., 242., 241.),
        BIRCH => {
            if ((by * 7 + ty / 2 + bx * 2) % 13 == 0 || ty % 8 == 0 && tx > 9) && tex > 0.19 {
                V(66., 62., 51.)
            } else {
                V(222., 221., 201.)
            }
        }
        WOOD => {
            let stripe = hash(bx * 16 + tx, 1, bz) * 0.3 + 0.82;
            V(115., 88., 48.) * stripe
        }
        LEAF => {
            let fleck = if tex > 0.70 {
                1.20
            } else if tex < 0.17 {
                0.70
            } else {
                1.
            };
            V(76. + block * 15., 145. + block * 17., 30.) * fleck
        }
        CLOUD => V(248., 250., 247.),
        HERB => V(128., 165., 48.) * (0.86 + v * 0.27),
        FLOWER => {
            if v > 0.57 {
                if block > 0.55 {
                    V(240., 239., 210.)
                } else if block > 0.2 {
                    V(108., 165., 222.)
                } else {
                    V(235., 194., 70.)
                }
            } else {
                V(93., 138., 38.)
            }
        }
        _ => V(120., 140., 80.),
    };
    if h.id == CLOUD {
        c
    } else {
        c * variation
    }
}
fn shade(w: &World, h: Hit, view: V, reflection: bool) -> V {
    let sun = V(-0.47, 0.78, -0.34).unit();
    let nd = h.n.dot(sun).max(0.);
    let mut shadow = 1.;
    if nd > 0.01 && h.id != CLOUD {
        if let Some(sh) = w.ray(h.p + h.n * 0.015, sun, 110.) {
            if sh.id != CLOUD {
                shadow = if sh.id == LEAF { 0.43 } else { 0.27 };
            }
        }
    }
    let ambient = if h.n.1 > 0.5 {
        0.68
    } else if h.n.1 < -0.5 {
        0.44
    } else {
        0.57
    };
    let mut light = ambient + nd * 0.52 * shadow;
    // Local contact occlusion at shared block corners, without inked outlines.
    if h.id != CLOUD {
        let q = h.p + h.n * 0.04;
        let ax = if h.n.1.abs() > 0.5 {
            V(0.65, 0., 0.)
        } else {
            V(0., 0.65, 0.)
        };
        let az = if h.n.2.abs() > 0.5 {
            V(0.65, 0., 0.)
        } else {
            V(0., 0., 0.65)
        };
        let mut occ = 0.;
        for a in [ax, ax * (-1.), az, az * (-1.)] {
            let r = q + a;
            if w.get(r.0.floor() as i32, r.1.floor() as i32, r.2.floor() as i32) != AIR {
                occ += 1.;
            }
        }
        light *= 1. - occ * 0.045;
    }
    let mut c = albedo(h) * light;
    if h.id == CLOUD {
        c = albedo(h) * (if h.n.1 < 0. { 0.90 } else { 0.95 + nd * 0.06 });
    }
    // Warm sunlight, cool open-sky fill, atmospheric perspective.
    c = V(
        c.0 * (1. + nd * shadow * 0.035),
        c.1,
        c.2 * (1. - nd * shadow * 0.045),
    );
    let fog = (1. - (-h.t / 360.).exp()).powf(1.65) * if reflection { 0.78 } else { 0.88 };
    let horizon = sky(V(view.0, 0.025, view.2).unit());
    c.lerp(horizon, fog)
}
fn trace(w: &World, o: V, d: V) -> V {
    let hit = w.ray(o, d, 850.);
    let tw = if d.1 < -0.0001 {
        (WATER - o.1) / d.1
    } else {
        f64::INFINITY
    };
    if tw > 0. && tw < hit.map_or(f64::INFINITY, |h| h.t) {
        let p = o + d * tw;
        if p.2 >= 0.
            && p.2 < NZ as f64
            && p.0 >= 0.
            && p.0 < NX as f64
            && elevation(p.0 - 192., p.2) < 13
        {
            // Small broad ripples distort a real reflected ray through the voxel world.
            let a = (p.0 * 2.8 + p.2 * 3.7).sin() * 0.009 + (p.0 * 7.1 - p.2 * 4.2).sin() * 0.004;
            let b = (p.0 * 2.2 - p.2 * 4.1).cos() * 0.013;
            let normal = V(a, 1., b).unit();
            let rd = (d - normal * (2. * d.dot(normal))).unit();
            let reflect = w
                .ray(p + V(0., 0.035, 0.), rd, 600.)
                .map_or_else(|| sky(rd), |h| shade(w, h, rd, true));
            let deep = V(40., 116., 139.);
            let bottom = hit.map_or(deep, |h| shade(w, h, d, false).lerp(deep, 0.64));
            let fresnel = 0.22 + 0.76 * (1. - (-d.dot(normal)).max(0.)).powf(4.);
            let rip = (p.0 * 0.9 + p.2 * 2.3).sin() * (p.0 * 1.7 - p.2 * 0.4).sin();
            let mut c = bottom.lerp(reflect, fresnel) + V(5., 9., 9.) * rip.max(0.).powf(14.);
            let sun = V(-0.47, 0.78, -0.34).unit();
            let sparkle = rd.dot(sun).max(0.).powf(160.);
            c = c + V(76., 68., 43.) * sparkle;
            return c.lerp(sky(V(d.0, 0.015, d.2).unit()), (tw / 850.).min(0.28));
        }
    }
    hit.map_or_else(|| sky(d), |h| shade(w, h, d, false))
}
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    let ppm = args.get(1).map_or("target/scenic-home.ppm", String::as_str);
    let width = args
        .get(3)
        .and_then(|s| s.parse::<usize>().ok())
        .unwrap_or(1920);
    let height = args
        .get(4)
        .and_then(|s| s.parse::<usize>().ok())
        .unwrap_or(1080);
    if width == 0 || height == 0 {
        return Err("image dimensions must be positive".into());
    }
    eprintln!("Building original voxel river valley...");
    let world = Arc::new(build());
    let camera = V(207., 25., 17.);
    let forward = V(-0.045, 0.012, 1.).unit();
    let right = V(0., 1., 0.).cross(forward).unit();
    let up = forward.cross(right).unit();
    let fov = (65f64.to_radians() * 0.5).tan();
    let threads = thread::available_parallelism()
        .map_or(8, |n| n.get())
        .min(24);
    let mut workers = Vec::new();
    for worker in 0..threads {
        let w = world.clone();
        workers.push(thread::spawn(move || {
            let mut rows = Vec::new();
            for y in (worker..height).step_by(threads) {
                let mut row = Vec::with_capacity(width * 3);
                for x in 0..width {
                    let mut c = V::default();
                    for (jx, jy) in [(0.25, 0.25), (0.75, 0.75)] {
                        let px = (2. * (x as f64 + jx) / width as f64 - 1.) * width as f64
                            / height as f64
                            * fov;
                        let py = (1. - 2. * (y as f64 + jy) / height as f64) * fov;
                        let ray = (forward + right * px + up * py).unit();
                        c = c + trace(&w, camera, ray) * 0.5;
                    }
                    row.extend_from_slice(&c.rgb());
                }
                rows.push((y, row));
            }
            rows
        }));
    }
    let mut pixels = vec![0u8; width * height * 3];
    for worker in workers {
        for (y, row) in worker.join().map_err(|_| "render thread panicked")? {
            pixels[y * width * 3..(y + 1) * width * 3].copy_from_slice(&row);
        }
    }
    let mut file = BufWriter::new(File::create(ppm)?);
    write!(file, "P6\n{width} {height}\n255\n")?;
    file.write_all(&pixels)?;
    file.flush()?;
    eprintln!("Wrote {ppm} ({width} × {height})");
    #[cfg(feature = "png")]
    if let Some(png) = args.get(2) {
        image::save_buffer_with_format(
            png,
            &pixels,
            width as u32,
            height as u32,
            image::ColorType::Rgb8,
            image::ImageFormat::Png,
        )?;
        eprintln!("Wrote {png}");
    }
    Ok(())
}
