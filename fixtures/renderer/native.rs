use oxide_renderer_fixture::fixture::{render_svg, write_png};
use std::path::{Path, PathBuf};

fn collect_cases(root: &Path, paths: &mut Vec<PathBuf>) {
    for entry in std::fs::read_dir(root).unwrap() {
        let path = entry.unwrap().path();
        if path.is_dir() {
            collect_cases(&path, paths);
        } else if path.extension().is_some_and(|ext| ext == "mmd") {
            paths.push(path);
        }
    }
}

fn main() {
    let mut args = std::env::args_os().skip(1);
    let cases = PathBuf::from(args.next().expect("cases directory"));
    let output = PathBuf::from(args.next().expect("output directory"));
    let mut paths = Vec::new();
    collect_cases(&cases, &mut paths);
    paths.sort();
    assert!(!paths.is_empty(), "no renderer cases");
    let count = paths.len();
    for input in paths {
        let name = input.strip_prefix(&cases).unwrap();
        let source = std::fs::read(&input).unwrap();
        let size = unsafe { render_svg(source.as_ptr(), source.len(), core::ptr::null_mut(), 0) };
        let mut svg = vec![0; size];
        let written = unsafe { render_svg(source.as_ptr(), source.len(), svg.as_mut_ptr(), svg.len()) };
        assert_eq!(written, size, "{}: SVG length changed", name.display());
        let mut second = vec![0; size];
        let written = unsafe { render_svg(source.as_ptr(), source.len(), second.as_mut_ptr(), second.len()) };
        assert_eq!(written, size, "{}: SVG length changed", name.display());
        assert_eq!(svg, second, "{}: native SVG is not deterministic", name.display());
        let svg_path = output.join(name).with_extension("svg");
        std::fs::create_dir_all(svg_path.parent().unwrap()).unwrap();
        std::fs::write(svg_path, svg).unwrap();
        let png = output.join(name).with_extension("png");
        let path = png.to_str().unwrap();
        unsafe { write_png(source.as_ptr(), source.len(), path.as_ptr(), path.len()) };
        let first = std::fs::read(&png).unwrap();
        std::fs::remove_file(&png).unwrap();
        unsafe { write_png(source.as_ptr(), source.len(), path.as_ptr(), path.len()) };
        let second = std::fs::read(&png).unwrap();
        assert_eq!(first, second, "{}: native PNG is not deterministic", name.display());
        println!("{}: {size} SVG bytes, {} PNG bytes", name.display(), second.len());
    }
    println!("{count} cases: native SVG and PNG generation and determinism passed");
}
