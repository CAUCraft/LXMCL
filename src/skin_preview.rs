//! Small orthographic skin preview. Uses the original atlas, nearest-neighbor
//! sampling and a depth buffer; no web view, network request or AI asset is used.
use slint::{Image, Rgba8Pixel, SharedPixelBuffer};

const WIDTH: u32 = 280;
const HEIGHT: u32 = 400;

#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Pose {
    pub body_yaw: f32,
    pub head_yaw: f32,
    pub head_pitch: f32,
}

impl Default for Pose {
    fn default() -> Self {
        Self {
            body_yaw: -24.,
            head_yaw: 0.,
            head_pitch: 0.,
        }
    }
}

impl Pose {
    pub fn towards(x: f32, y: f32) -> Self {
        // atan gives a soft limit even when the pointer is far from the character.
        let horizontal = x.atan() / std::f32::consts::FRAC_PI_2;
        let vertical = y.atan() / std::f32::consts::FRAC_PI_2;
        Self {
            body_yaw: horizontal * 24.,
            head_yaw: horizontal * 34.,
            head_pitch: vertical * 26.,
        }
    }

    pub fn settle_towards(&mut self, target: Self, dt: f32) -> bool {
        let mut moving = false;
        for (current, goal, response) in [
            (&mut self.body_yaw, target.body_yaw, 0.095),
            (&mut self.head_yaw, target.head_yaw, 0.055),
            (&mut self.head_pitch, target.head_pitch, 0.055),
        ] {
            *current += (goal - *current) * (1. - (-dt / response).exp());
            if (goal - *current).abs() < 0.12 {
                *current = goal;
            } else {
                moving = true;
            }
        }
        moving
    }
}

#[derive(Clone, Copy)]
struct Vertex {
    x: f32,
    y: f32,
    z: f32,
    u: f32,
    v: f32,
}

pub struct SkinPreview {
    pixels: image::RgbaImage,
    outer: image::RgbaImage,
}

impl SkinPreview {
    pub fn new() -> Self {
        let source = image::load_from_memory(include_bytes!("../ui/assets/luanxiao.png"))
            .expect("Bundled club skin must be a readable image");
        let pixels = source.to_rgba8();
        assert_eq!(
            pixels.dimensions(),
            (64, 64),
            "Expected a 64 × 64 skin atlas"
        );
        let outer = pixels.clone();
        Self { pixels, outer }
    }

    pub fn render(&self, angle: i32) -> Image {
        self.render_pose(Pose {
            body_yaw: angle as f32,
            head_yaw: 0.,
            head_pitch: 0.,
        })
    }

    pub fn render_pose(&self, pose: Pose) -> Image {
        let pixels = self.render_pixels(pose);
        Image::from_rgba8(SharedPixelBuffer::<Rgba8Pixel>::clone_from_slice(
            &pixels, WIDTH, HEIGHT,
        ))
    }

    fn render_pixels(&self, pose: Pose) -> Vec<u8> {
        let mut target = vec![0; (WIDTH * HEIGHT * 4) as usize];
        let mut depth = vec![f32::NEG_INFINITY; (WIDTH * HEIGHT) as usize];
        let (sy, cy) = pose.body_yaw.to_radians().sin_cos();
        let (sp, cp) = 9_f32.to_radians().sin_cos();
        let (shy, chy) = pose.head_yaw.to_radians().sin_cos();
        let (shp, chp) = pose.head_pitch.to_radians().sin_cos();
        let project = |mut p: [f32; 3], u: f32, v: f32, head: bool| {
            if head {
                // Rotate the head around the neck, then apply the body's yaw.
                let y = (p[1] - 24.) * chp - p[2] * shp;
                let z = (p[1] - 24.) * shp + p[2] * chp;
                p = [p[0] * chy + z * shy, y + 24., -p[0] * shy + z * chy];
            }
            let x = p[0] * cy + p[2] * sy;
            let z = -p[0] * sy + p[2] * cy;
            let y = (p[1] - 16.) * cp - z * sp;
            let z = (p[1] - 16.) * sp + z * cp;
            Vertex {
                x: WIDTH as f32 / 2. + x * 10.5,
                y: HEIGHT as f32 / 2. - y * 10.5,
                z,
                u,
                v,
            }
        };
        // Atlas origins and dimensions follow the standard Minecraft 64px skin layout.
        // Both shells use the same joint transform; UV sizes stay unexpanded.
        for (center, size, origin, inflate) in [
            ([0., 28., 0.], [8., 8., 8.], [0., 0.], 0.),
            ([0., 18., 0.], [8., 12., 4.], [16., 16.], 0.),
            ([-6., 18., 0.], [4., 12., 4.], [40., 16.], 0.),
            ([6., 18., 0.], [4., 12., 4.], [32., 48.], 0.),
            ([-2., 6., 0.], [4., 12., 4.], [0., 16.], 0.),
            ([2., 6., 0.], [4., 12., 4.], [16., 48.], 0.),
            ([0., 28., 0.], [8., 8., 8.], [32., 0.], 0.5),
            ([0., 18., 0.], [8., 12., 4.], [16., 32.], 0.25),
            ([-6., 18., 0.], [4., 12., 4.], [40., 32.], 0.25),
            ([6., 18., 0.], [4., 12., 4.], [48., 48.], 0.25),
            ([-2., 6., 0.], [4., 12., 4.], [0., 32.], 0.25),
            ([2., 6., 0.], [4., 12., 4.], [0., 48.], 0.25),
        ] {
            let [w, h, d] = size;
            let [u, v] = origin;
            let [cx, cy, cz] = center;
            let (l, r, b, t, f, k) = (
                cx - w / 2. - inflate,
                cx + w / 2. + inflate,
                cy - h / 2. - inflate,
                cy + h / 2. + inflate,
                cz + d / 2. + inflate,
                cz - d / 2. - inflate,
            );
            let faces = [
                // Vertices run from top-left to top-right to bottom-right to bottom-left.
                (
                    [[l, t, f], [r, t, f], [r, b, f], [l, b, f]],
                    [u + d, v + d, w, h],
                    1.0,
                ),
                (
                    [[r, t, k], [l, t, k], [l, b, k], [r, b, k]],
                    [u + 2. * d + w, v + d, w, h],
                    0.84,
                ),
                (
                    [[l, t, k], [l, t, f], [l, b, f], [l, b, k]],
                    [u, v + d, d, h],
                    0.88,
                ),
                (
                    [[r, t, f], [r, t, k], [r, b, k], [r, b, f]],
                    [u + d + w, v + d, d, h],
                    0.78,
                ),
                (
                    [[l, t, k], [r, t, k], [r, t, f], [l, t, f]],
                    [u + d, v, w, d],
                    1.06,
                ),
                (
                    [[l, b, f], [r, b, f], [r, b, k], [l, b, k]],
                    [u + d + w, v, w, d],
                    0.68,
                ),
            ];
            for (corners, uv, shade) in faces {
                let [u, v, tw, th] = uv;
                let verts = [
                    project(corners[0], u, v, cy == 28.),
                    project(corners[1], u + tw, v, cy == 28.),
                    project(corners[2], u + tw, v + th, cy == 28.),
                    project(corners[3], u, v + th, cy == 28.),
                ];
                self.triangle(
                    [verts[0], verts[1], verts[2]],
                    shade,
                    inflate > 0.,
                    &mut target,
                    &mut depth,
                );
                self.triangle(
                    [verts[0], verts[2], verts[3]],
                    shade,
                    inflate > 0.,
                    &mut target,
                    &mut depth,
                );
            }
        }
        target
    }

    fn triangle(
        &self,
        verts: [Vertex; 3],
        shade: f32,
        outer: bool,
        target: &mut [u8],
        depth: &mut [f32],
    ) {
        let [a, b, c] = verts;
        let edge = |a: Vertex, b: Vertex, x: f32, y: f32| {
            (x - a.x) * (b.y - a.y) - (y - a.y) * (b.x - a.x)
        };
        let area = edge(a, b, c.x, c.y);
        // All faces use clockwise screen-space winding. Cull hidden faces before
        // rasterizing: pointer motion must not spend time drawing the back of each cube.
        if area >= -0.001 {
            return;
        }
        let min_x = a.x.min(b.x).min(c.x).floor().max(0.) as u32;
        let max_x = a.x.max(b.x).max(c.x).ceil().min((WIDTH - 1) as f32) as u32;
        let min_y = a.y.min(b.y).min(c.y).floor().max(0.) as u32;
        let max_y = a.y.max(b.y).max(c.y).ceil().min((HEIGHT - 1) as f32) as u32;
        for y in min_y..=max_y {
            for x in min_x..=max_x {
                let px = x as f32 + 0.5;
                let py = y as f32 + 0.5;
                let wa = edge(b, c, px, py) / area;
                let wb = edge(c, a, px, py) / area;
                let wc = 1. - wa - wb;
                if wa < -0.0001 || wb < -0.0001 || wc < -0.0001 {
                    continue;
                }
                let z = wa * a.z + wb * b.z + wc * c.z;
                let index = (y * WIDTH + x) as usize;
                if z <= depth[index] {
                    continue;
                }
                let u = (wa * a.u + wb * b.u + wc * c.u).floor().clamp(0., 63.) as u32;
                let v = (wa * a.v + wb * b.v + wc * c.v).floor().clamp(0., 63.) as u32;
                let color = if outer { &self.outer } else { &self.pixels }
                    .get_pixel(u, v)
                    .0;
                if color[3] == 0 {
                    continue;
                }
                depth[index] = z;
                for channel in 0..3 {
                    target[index * 4 + channel] =
                        (color[channel] as f32 * shade).clamp(0., 255.) as u8;
                }
                target[index * 4 + 3] = color[3];
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn outer_layer_is_visible_and_base_white_is_preserved() {
        let mut skin = SkinPreview::new();
        // Preserve opaque white details as well as genuine transparent pixels.
        assert!(
            skin.outer
                .pixels()
                .any(|p| p.0[..3] == [255, 255, 255] && p[3] == 255)
        );
        assert_eq!(skin.pixels.get_pixel(63, 63)[3], 0);
        assert_eq!(skin.outer.get_pixel(63, 63)[3], 0);
        let layered = skin.render_pixels(Pose::default());
        for p in skin.outer.pixels_mut() {
            p[3] = 0;
        }
        assert_ne!(layered, skin.render_pixels(Pose::default()));
    }

    #[test]
    fn avatar_has_transparent_margin_and_distinct_views() {
        let skin = SkinPreview::new();
        let front = skin.render_pixels(Pose {
            body_yaw: 0.,
            ..Pose::default()
        });
        let back = skin.render_pixels(Pose {
            body_yaw: 180.,
            ..Pose::default()
        });
        assert_ne!(front, back);
        assert!(front.chunks_exact(4).filter(|p| p[3] != 0).count() > 20_000);
        assert!(front.chunks_exact(4).filter(|p| p[3] == 0).count() > 40_000);
        assert!(front[..WIDTH as usize * 4].iter().all(|v| *v == 0));
    }

    #[test]
    fn head_articulates_without_rotating_the_lower_body() {
        let skin = SkinPreview::new();
        let neutral = skin.render_pixels(Pose::default());
        for (yaw, pitch) in [(-34., -26.), (34., 26.)] {
            let turned = skin.render_pixels(Pose {
                head_yaw: yaw,
                head_pitch: pitch,
                ..Pose::default()
            });
            assert_ne!(neutral, turned);
            // Below the shoulders, only the unchanged torso/limbs can contribute.
            assert_eq!(
                &neutral[180 * WIDTH as usize * 4..],
                &turned[180 * WIDTH as usize * 4..]
            );
            assert!(turned[..WIDTH as usize * 4].iter().all(|v| *v == 0));
        }
    }

    #[test]
    fn tracking_is_bounded_and_settles_after_retargeting() {
        let left = Pose::towards(-1000., -1000.);
        let right = Pose::towards(1000., 1000.);
        assert!(left.body_yaw < 0. && right.body_yaw > 0.);
        assert!(right.body_yaw < 24. && right.head_yaw < 34. && right.head_pitch < 26.);
        let mut pose = left;
        assert!(pose.settle_towards(right, 0.016));
        // A reversal replaces the old target, and returning to rest stops exactly.
        for _ in 0..90 {
            pose.settle_towards(Pose::default(), 0.016);
        }
        assert_eq!(pose, Pose::default());
        assert!(!pose.settle_towards(Pose::default(), 0.016));
    }

    #[test]
    #[ignore = "manual render timing, not a timing assertion"]
    fn benchmark_tracking_frames() {
        let skin = SkinPreview::new();
        let mut times = Vec::new();
        for i in 0..360 {
            let t = i as f32 * 0.07;
            let start = std::time::Instant::now();
            std::hint::black_box(skin.render_pixels(Pose::towards(t.sin() * 3., t.cos() * 2.)));
            times.push(start.elapsed().as_secs_f64() * 1000.);
        }
        times.sort_by(f64::total_cmp);
        println!(
            "280x400 articulated skin CPU raster (360 poses): p50={:.3} ms, p95={:.3} ms, max={:.3} ms",
            times[180], times[342], times[359]
        );
    }
}
