fn main() {
    if let Err(error) = mermaid_rs_renderer::run() {
        eprintln!("error: {error}");
        std::process::exit(1);
    }
}
