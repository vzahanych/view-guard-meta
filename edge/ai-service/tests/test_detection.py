"""
Unit tests for detection logic.
"""

import pytest
from ai_service.detection import DetectionLogic, DetectionFilter
from ai_service.inference import BoundingBox, DetectionResult


class TestDetectionFilter:
    """Tests for DetectionFilter."""
    
    def test_filter_initialization(self):
        """Test filter initialization."""
        filter_config = DetectionFilter()
        assert filter_config.enabled_classes is None
        assert filter_config.min_confidence == 0.5
        assert filter_config.min_area == 0.0
    
    def test_filter_with_classes(self):
        """Test filter with enabled classes."""
        filter_config = DetectionFilter(enabled_classes={"person", "car"})
        assert filter_config.enabled_classes == {"person", "car"}


class TestDetectionLogic:
    """Tests for DetectionLogic."""
    
    @pytest.fixture
    def sample_result(self):
        """Create sample detection result for testing."""
        boxes = [
            BoundingBox(10, 10, 50, 50, 0.9, 0, "person"),
            BoundingBox(100, 100, 150, 150, 0.8, 2, "car"),
            BoundingBox(200, 200, 250, 250, 0.7, 5, "bus"),
            BoundingBox(300, 300, 350, 350, 0.3, 0, "person"),  # Low confidence
        ]
        return DetectionResult(
            bounding_boxes=boxes,
            inference_time_ms=10.0,
            frame_shape=(480, 640),
            model_input_shape=(640, 640),
        )
    
    def test_logic_initialization(self):
        """Test detection logic initialization."""
        logic = DetectionLogic()
        assert logic.filter is not None
        assert logic.PERSON_CLASS_ID == 0
        assert 2 in logic.VEHICLE_CLASS_IDS  # CAR
    
    def test_detect_persons(self, sample_result):
        """Test person detection."""
        logic = DetectionLogic()
        persons = logic.detect_persons(sample_result)
        
        assert len(persons) == 2  # Two person boxes
        assert all(box.class_name == "person" for box in persons)
    
    def test_detect_vehicles(self, sample_result):
        """Test vehicle detection."""
        logic = DetectionLogic()
        vehicles = logic.detect_vehicles(sample_result)
        
        assert len(vehicles) == 2  # car and bus
        assert all(box.class_id in logic.VEHICLE_CLASS_IDS for box in vehicles)
    
    def test_detect_custom_classes(self, sample_result):
        """Test custom class detection."""
        logic = DetectionLogic()
        custom = logic.detect_custom_classes(sample_result, ["person", "car"])
        
        assert len(custom) == 3  # 2 persons + 1 car
        assert all(box.class_name in ["person", "car"] for box in custom)
    
    def test_has_person(self, sample_result):
        """Test has_person check."""
        logic = DetectionLogic()
        assert logic.has_person(sample_result) is True
    
    def test_has_vehicle(self, sample_result):
        """Test has_vehicle check."""
        logic = DetectionLogic()
        assert logic.has_vehicle(sample_result) is True
    
    def test_has_person_false(self):
        """Test has_person returns False when no persons."""
        logic = DetectionLogic()
        result = DetectionResult(
            bounding_boxes=[BoundingBox(10, 10, 50, 50, 0.9, 2, "car")],
            inference_time_ms=10.0,
            frame_shape=(480, 640),
            model_input_shape=(640, 640),
        )
        assert logic.has_person(result) is False
    
    def test_filter_detections_confidence(self, sample_result):
        """Test filtering by confidence threshold."""
        logic = DetectionLogic(
            detection_filter=DetectionFilter(min_confidence=0.5)
        )
        
        filtered = logic.filter_detections(sample_result)
        
        # Should filter out low confidence box (0.3)
        assert len(filtered.bounding_boxes) == 3
        assert all(box.confidence >= 0.5 for box in filtered.bounding_boxes)
    
    def test_filter_detections_classes(self, sample_result):
        """Test filtering by enabled classes."""
        logic = DetectionLogic(
            detection_filter=DetectionFilter(enabled_classes={"person"})
        )
        
        filtered = logic.filter_detections(sample_result)
        
        # Should only keep person boxes
        assert len(filtered.bounding_boxes) == 2
        assert all(box.class_name == "person" for box in filtered.bounding_boxes)
    
    def test_filter_detections_area(self, sample_result):
        """Test filtering by area constraints."""
        logic = DetectionLogic(
            detection_filter=DetectionFilter(min_area=1000.0, max_area=5000.0)
        )
        
        filtered = logic.filter_detections(sample_result)
        
        # All boxes should have area between min and max
        for box in filtered.bounding_boxes:
            area = (box.x2 - box.x1) * (box.y2 - box.y1)
            assert 1000.0 <= area <= 5000.0
    
    def test_get_detection_summary(self, sample_result):
        """Test getting detection summary."""
        logic = DetectionLogic()
        summary = logic.get_detection_summary(sample_result)
        
        assert "person" in summary
        assert "car" in summary
        assert "bus" in summary
        assert summary["person"]["count"] == 2
        assert summary["car"]["count"] == 1
    
    def test_set_confidence_threshold(self):
        """Test setting confidence threshold."""
        logic = DetectionLogic()
        logic.set_confidence_threshold(0.7)
        
        assert logic.filter.min_confidence == 0.7
    
    def test_set_confidence_threshold_invalid(self):
        """Test setting invalid confidence threshold."""
        logic = DetectionLogic()
        
        with pytest.raises(ValueError, match="Confidence threshold must be between"):
            logic.set_confidence_threshold(1.5)
    
    def test_set_enabled_classes(self):
        """Test setting enabled classes."""
        logic = DetectionLogic()
        logic.set_enabled_classes(["person", "car"])
        
        assert logic.filter.enabled_classes == {"person", "car"}
    
    def test_set_enabled_classes_none(self):
        """Test setting enabled classes to None (all classes)."""
        logic = DetectionLogic()
        logic.set_enabled_classes(["person"])
        logic.set_enabled_classes(None)
        
        assert logic.filter.enabled_classes is None

