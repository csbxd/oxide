#[path = "process.rs"]
mod fixture;

fn main() {
    if let Ok(code) = std::env::var("OXIDE_PROCESS_EXIT") {
        if !code.is_empty() {
            fixture::exit_probe(code.parse().unwrap());
        }
    }
    println!("{}", fixture::args_digest());
}
