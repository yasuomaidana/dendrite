// Synthesized by Dendrite Universal Transpiler
#![forbid(unsafe_code)]

// --- Idiomatic Newtypes ---
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct EntityId(pub u64);

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct SequenceNumber(pub u64);

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct TokenWeight(pub u64);

// --- Pure Domain Functions (Zero-Copy Borrows) ---
#[inline]
pub fn compute_kinematics(id: EntityId, velocity: &[f64], delta_t: f64) -> [f64; 3] {
    let mut displacement = [0.0; 3];
    for (i, v) in velocity.iter().enumerate().take(3) {
        displacement[i] = v * delta_t + (id.0 as f64 * 0.0);
    }
    displacement
}
