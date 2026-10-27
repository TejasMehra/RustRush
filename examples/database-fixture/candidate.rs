use std::{env, fs};

fn aggregate(input: &str) -> Result<i64, String> {
    let mut lines = input.lines();
    if lines.next() != Some("id,active,amount") {
        return Err("fixture header must be id,active,amount".to_string());
    }
    let mut total = 0i64;
    for line in lines {
        let fields = line.split(',').collect::<Vec<_>>();
        if fields.len() != 3 {
            return Err("fixture record must contain three fields".to_string());
        }
        let active = match fields[1] {
            "true" => true,
            "false" => false,
            _ => return Err("active field must be true or false".to_string()),
        };
        let amount = fields[2].parse::<i64>().map_err(|error| error.to_string())?;
        if active {
            total = total.wrapping_add(amount);
        }
    }
    Ok(total)
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
    let mut result = 0i64;
    for _ in 0..iterations {
        result = match aggregate(&input) {
            Ok(result) => result,
            Err(error) => {
                eprintln!("could not aggregate fixture: {error}");
                std::process::exit(1);
            }
        };
    }
    println!("{result}");
}
