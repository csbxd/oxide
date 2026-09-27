#![no_std]
pub fn observe() -> u32 {
    let _ = facade::RenamedTuple(3, 5);
    let _ = facade::ChoiceAlias::Tuple(7);
    let point = facade::RenamedPoint::new(facade::alias());
    let cloned = <facade::RenamedPoint as Clone>::clone(&point);
    let default = <facade::PointAlias as Default>::default();
    facade::Measure::default_method(&cloned)
        + default.value()
        + facade::nested::renamed()
        + facade::renamed_module::same()
        + facade::Byte::specialized()
        + facade::cycle::again::leaf()
}
