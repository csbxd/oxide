//! Re-export the renderer's complete library API, retaining its public modules,
//! types and methods. Test observers live separately and do not shadow its API.

pub use mermaid_rs_renderer::*;

pub mod api;
pub mod fixture;
