//! Native comparison for generated_test.go's warmed SVG/PNG benchmarks.
//! Arguments: cases directory, native reference directory, output directory.

use oxide_renderer_fixture::fixture::{render_svg, write_png};
use std::hint::black_box;
use std::path::PathBuf;
use std::time::Instant;

const ITERATIONS: u32 = 3;

fn svg(source: &[u8], buffer: &mut [u8]) {
    let size = unsafe {
        render_svg(
            black_box(source.as_ptr()),
            source.len(),
            buffer.as_mut_ptr(),
            buffer.len(),
        )
    };
    assert_eq!(size, buffer.len(), "SVG length differs");
}

fn main() {
    let mut args = std::env::args_os().skip(1);
    let cases = PathBuf::from(args.next().expect("cases directory"));
    let reference = PathBuf::from(args.next().expect("native reference directory"));
    let output = PathBuf::from(args.next().expect("output directory"));
    assert!(args.next().is_none(), "unexpected argument");
    std::fs::create_dir_all(&output).unwrap();

    println!("format,case,run,iterations,ns_per_op");
    for format in ["SVG", "PNG"] {
        for name in ["flowchart", "sequence", "class"] {
            let source = std::fs::read(cases.join(format!("{name}.mmd"))).unwrap();
            let want =
                std::fs::read(reference.join(format!("{name}.{}", format.to_lowercase()))).unwrap();
            for run in 1..=3 {
                if format == "SVG" {
                    let mut buffer = vec![0; want.len()];
                    svg(&source, &mut buffer);
                    assert!(buffer == want, "{name}: SVG bytes differ before timing");
                    let start = Instant::now();
                    for _ in 0..ITERATIONS {
                        svg(&source, &mut buffer);
                    }
                    let elapsed = start.elapsed();
                    assert!(buffer == want, "{name}: SVG bytes differ after timing");
                    println!(
                        "SVG,{name},{run},{ITERATIONS},{}",
                        elapsed.as_nanos() / u128::from(ITERATIONS)
                    );
                } else {
                    let path = output.join(format!("benchmark-{name}.png"));
                    if path.exists() {
                        std::fs::remove_file(&path).unwrap();
                    }
                    let filename = path.to_str().expect("UTF-8 output path");
                    let render = || unsafe {
                        write_png(
                            black_box(source.as_ptr()),
                            source.len(),
                            filename.as_ptr(),
                            filename.len(),
                        )
                    };
                    render();
                    assert!(
                        std::fs::read(&path).unwrap() == want,
                        "{name}: PNG bytes differ before timing"
                    );
                    let start = Instant::now();
                    for _ in 0..ITERATIONS {
                        render();
                    }
                    let elapsed = start.elapsed();
                    assert!(
                        std::fs::read(&path).unwrap() == want,
                        "{name}: PNG bytes differ after timing"
                    );
                    println!(
                        "PNG,{name},{run},{ITERATIONS},{}",
                        elapsed.as_nanos() / u128::from(ITERATIONS)
                    );
                }
            }
        }
    }
}
