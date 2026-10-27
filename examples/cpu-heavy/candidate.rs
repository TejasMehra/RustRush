use std::env;

fn main() {
    let mut args = env::args().skip(1);
    let mut iterations: u64 = 0;
    while let Some(argument) = args.next() {
        if argument == "--iterations" {
            iterations = args.next().and_then(|value| value.parse().ok()).unwrap_or(0);
        }
    }
    if iterations == 0 {
        eprintln!("--iterations must be a positive integer");
        std::process::exit(2);
    }

    let mut value: u64 = 0x9e3779b97f4a7c15;
    for _ in 0..iterations {
        value = value.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
        value ^= value >> 29;
        value = value.wrapping_mul(0xbf58476d1ce4e5b9);
        value ^= value >> 31;
    }
    println!("{value}");
}
