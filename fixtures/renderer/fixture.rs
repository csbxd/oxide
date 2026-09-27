//! Native-only observation helpers. This module is never a translation input.

unsafe fn render(source: *const u8, source_len: usize) -> String {
    let source = unsafe { core::slice::from_raw_parts(source, source_len) };
    let source = core::str::from_utf8(source).expect("fixture source is UTF-8");
    mermaid_rs_renderer::render(source).expect("render fixture SVG")
}

/// Return the complete SVG byte length. If capacity is sufficient, copy every
/// byte into the caller's buffer. A null output is permitted at capacity 0.
///
/// # Safety
/// source must name source_len readable bytes. When capacity is nonzero,
/// output must name that many writable bytes, separate from source.
pub unsafe fn render_svg(
    source: *const u8,
    source_len: usize,
    output: *mut u8,
    capacity: usize,
) -> usize {
    let svg = unsafe { render(source, source_len) };
    if capacity != 0 && capacity >= svg.len() {
        unsafe { core::ptr::copy_nonoverlapping(svg.as_ptr(), output, svg.len()) };
    }
    svg.len()
}

/// Call the original PNG output API, retaining the renderer's PNG call graph.
///
/// # Safety
/// source and path must name source_len and path_len readable UTF-8 bytes.
pub unsafe fn write_png(
    source: *const u8,
    source_len: usize,
    path: *const u8,
    path_len: usize,
) {
    let path = unsafe { core::slice::from_raw_parts(path, path_len) };
    let path = core::str::from_utf8(path).expect("fixture output path is UTF-8");
    mermaid_rs_renderer::write_output_png(
        &unsafe { render(source, source_len) },
        std::path::Path::new(path),
        &mermaid_rs_renderer::RenderConfig::default(),
        &mermaid_rs_renderer::Theme::modern(),
    )
    .expect("render fixture PNG");
}
