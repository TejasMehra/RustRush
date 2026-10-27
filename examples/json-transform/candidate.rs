use std::{env, fs};

fn transform(input: &str) -> Result<String, String> {
    let key = input.find("\"values\"").ok_or("missing values field")?;
    let array_start = key + input[key..].find('[').ok_or("missing values array")? + 1;
    let array_end = array_start + input[array_start..].find(']').ok_or("unterminated values array")?;
    let body = input[array_start..array_end].trim();
    let mut values = Vec::new();
    if !body.is_empty() {
        for value in body.split(',') {
            let parsed = value.trim().parse::<i64>().map_err(|error| error.to_string())?;
            values.push(parsed.wrapping_mul(2));
        }
    }
    let encoded = values.iter().map(i64::to_string).collect::<Vec<_>>().join(",");
    Ok(format!("{{\"values\":[{encoded}]}}"))
}

fn main() {
    let mut args = env::args().skip(1);
    let mut fixture = String::new();
    let mut iterations: u64 = 0;
    while let Some(argument) = args.next() {
        match argument.as_str() {
            "--fixture" => fixture = args.next().unwrap_or_default(),
            "--iterations" => iterations = args.next().and_then(|value| value.parse().ok()).unwrap_or(0),
            _ => {}
        }
    }
    if fixture.is_empty() || iterations == 0 {
        eprintln!("--fixture and a positive --iterations value are required");
        std::process::exit(2);
    }
    let input = match fs::read_to_string(fixture) {
        Ok(input) => input,
        Err(error) => {
            eprintln!("could not read fixture: {error}");
            std::process::exit(1);
        }
    };
    let mut output = String::new();
    for _ in 0..iterations {
        output = match transform(&input) {
            Ok(output) => output,
            Err(error) => {
                eprintln!("could not transform fixture: {error}");
                std::process::exit(1);
            }
        };
    }
    println!("{output}");
}
